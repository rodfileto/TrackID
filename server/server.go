// Package server assembles the HTTP application: it loads configuration, opens
// the database, wires the API router, mounts the prototype frontend, and runs
// the server. It is the composition root for cmd/trackid.
package server

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"github.com/rodfileto/trackid-vision/vision"

	"github.com/rodfileto/trackid/api"
	"github.com/rodfileto/trackid/ask"
	"github.com/rodfileto/trackid/assist"
	"github.com/rodfileto/trackid/cases"
	"github.com/rodfileto/trackid/fingerprint"
	"github.com/rodfileto/trackid/follows"
	"github.com/rodfileto/trackid/geocode"
	"github.com/rodfileto/trackid/internal/config"
	"github.com/rodfileto/trackid/internal/database"
	"github.com/rodfileto/trackid/internal/env"
	"github.com/rodfileto/trackid/internal/queue"
	"github.com/rodfileto/trackid/internal/web"
	"github.com/rodfileto/trackid/notifications"
	"github.com/rodfileto/trackid/realtime"
	"github.com/rodfileto/trackid/reports"
	"github.com/rodfileto/trackid/storage"
	"github.com/rodfileto/trackid/teams"
	"github.com/rodfileto/trackid/video"
)

// Run starts the API server on the configured port. The database is optional:
// when DATABASE_URL is empty the API still serves /health and the frontend, and
// every DB-backed route returns 503.
// Options are what an organization's own server adds to core's.
type Options struct {
	// Register adds the organization's routes, given the dependencies core built. protected
	// requires a signed-in, active user.
	Register func(deps api.Dependencies, engine *gin.Engine, protected *gin.RouterGroup)
}

// Run starts the API server and, when a build is present, the frontend. An organization runs its own
// routes through Options; cmd/trackid runs none.
func Run(options Options) error {
	env.Load()

	configuration := config.Load()
	if err := configuration.CheckProduction(config.ProcessAPI); err != nil {
		return err
	}
	if configuration.Production() {
		gin.SetMode(gin.ReleaseMode)
	}

	var db *sql.DB
	if configuration.DatabaseURL != "" {
		var err error
		db, err = database.Open(configuration.DatabaseURL)
		if err != nil {
			return err
		}
		defer db.Close()
	}

	var store *storage.Client
	if configuration.S3Endpoint != "" && configuration.S3AccessKey != "" &&
		configuration.S3SecretKey != "" && configuration.S3Bucket != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		var err error
		store, err = storage.NewClient(ctx, configuration.S3Endpoint, configuration.S3AccessKey, configuration.S3SecretKey, configuration.S3Bucket)
		cancel()
		if err != nil {
			store = nil
			configuration.Unavailable("object storage is configured but unreachable; evidence uploads will return 503: %v", err)
		}
	} else {
		log.Printf("object storage is not configured; evidence uploads will return 503")
	}

	var queueClient *asynq.Client
	if configuration.RedisURL != "" {
		var err error
		queueClient, err = queue.OpenClient(configuration.RedisURL)
		if err != nil {
			configuration.Unavailable("Redis is configured but the queue client could not be created; background jobs will not be enqueued: %v", err)
		} else {
			defer queueClient.Close()
		}
	} else {
		log.Printf("REDIS_URL is not configured; background jobs will not be enqueued")
	}

	// Assigned only on success: a nil *vision.Service stored in the interface
	// would not compare equal to nil, and the handler relies on that check.
	var faceVision cases.FaceVision
	var videoLab *video.Lab // the video lab, tracking faces with the same vision service
	if configuration.VisionDetectorPath != "" && configuration.VisionRecognizerPath != "" {
		vis, err := vision.NewService(vision.Config{
			Detector:          vision.DetectorKind(configuration.VisionDetector),
			DetectorPath:      configuration.VisionDetectorPath,
			RecognizerPath:    configuration.VisionRecognizerPath,
			SharedLibraryPath: configuration.VisionSharedLibraryPath,
			UseGPU:            configuration.VisionUseGPU,
		})
		if err != nil {
			configuration.Unavailable("vision service unavailable; face detection and on-the-fly face comparison will return 503: %v", err)
		} else {
			defer vis.Close()
			faceVision = vis
			videoLab = video.NewLab(video.VisionLabTracker(vis))
			defer videoLab.Close() // deletes the lab's temporary videos on shutdown
		}
	} else {
		log.Printf("VISION_DETECTOR_PATH/VISION_RECOGNIZER_PATH not configured; face detection and on-the-fly face comparison will return 503")
	}

	// The image lab compares its renders with SourceAFIS when the sidecar is configured and up.
	var fingerprintClient *fingerprint.Client
	if configuration.FingerprintSidecarURL != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		fp, err := fingerprint.NewClient(ctx, configuration.FingerprintSidecarURL)
		cancel()
		if err != nil {
			configuration.Unavailable("fingerprint sidecar unavailable; the image lab will not compare: %v", err)
		} else {
			fingerprintClient = fp
		}
	}

	// Addresses go through the census list in the database (cmd/load-cnefe), then Nominatim, then
	// the municipality (geocode.Layered, docs/geocoding-plan.md). One Nominatim client for the
	// process, so its pacing keeps the whole API within the server's rate limit. None unless
	// GEOCODER_URL names one; the public instance only when it says "public".
	var nominatim geocode.Geocoder
	if g, err := geocode.NewNominatim(geocode.NominatimConfig{
		BaseURL: configuration.GeocoderURL, UserAgent: configuration.GeocoderUserAgent,
		Email: configuration.GeocoderEmail, MinInterval: configuration.GeocoderMinInterval,
	}); errors.Is(err, geocode.ErrNoNominatim) {
		log.Printf("GEOCODER_URL not configured; addresses are found only in the census list and the municipalities")
	} else if err != nil {
		configuration.Unavailable("Nominatim unavailable; addresses are found only in the census list and the municipalities: %v", err)
	} else {
		nominatim = g
		if geocode.IsPublicNominatim(configuration.GeocoderURL) {
			log.Printf("GEOCODER_URL=public: addresses searched and points clicked that the census list can't answer are sent to %s", geocode.PublicNominatimURL)
		}
	}
	var geocoder geocode.Geocoder
	if db != nil || nominatim != nil {
		geocoder = geocode.NewLayered(db, nominatim)
	}

	// The assist lab's drafter: a local model that drafts registrations from uploaded files. Left
	// unconfigured, the lab still extracts text but cannot draft.
	var assistDrafter assist.Drafter
	if configuration.AssistEndpoint != "" && configuration.AssistModel != "" {
		assistDrafter = assist.NewOpenAIClientWithOptions(configuration.AssistEndpoint, configuration.AssistModel, assist.DefaultOccurrenceTypes, assist.OpenAIOptions{APIKey: configuration.AssistAPIKey, Timeout: configuration.AssistTimeout})
	} else {
		log.Printf("ASSIST_ENDPOINT/ASSIST_MODEL not configured; the assist lab will not draft")
	}

	// The AI mode's model. A configuration that makes no sense leaves it off, and says so, rather
	// than stopping the rest of the application.
	askModel, err := ask.NewModel(ask.Settings{
		Endpoint: configuration.AskEndpoint, Model: configuration.AskModel, APIKey: configuration.AskAPIKey,
		ExtraBody: configuration.AskExtraBody, Timeout: configuration.AskTimeout,
	})
	if err != nil {
		configuration.Unavailable("the AI mode is off: %v", err)
	} else if askModel == nil {
		log.Printf("ASK_ENDPOINT/ASK_MODEL not configured; the AI mode is off")
	} else {
		log.Printf("AI mode: model %s at %s (external: %t)", askModel.Name(), askModel.Host(), askModel.External())
	}

	signals, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Live updates: the hub pushes to browsers, and the listener brings in what the worker or any
	// other process committed (docs/realtime-plan.md). Neither needs Redis.
	// Teams tells users of their notifications through the webhook each sets (docs/follow-plan.md).
	var teamsService *teams.Service
	if db != nil {
		var err error
		teamsService, err = teams.New(db, teams.Settings{
			SecretsKey: configuration.SecretsKey, AppURL: configuration.AppURL,
			Hosts: teams.ParseHosts(configuration.TeamsWebhookHosts), Language: configuration.TeamsLanguage,
		})
		if err != nil && configuration.SecretsKey != "" {
			configuration.Unavailable("Teams messages are off: %v", err)
		} else if err != nil {
			log.Printf("Teams messages are off: %v", err)
		} else {
			teamsService.Run(signals)
			if configuration.AppURL == "" {
				log.Printf("APP_URL is not set; Teams messages will have no button to open the app")
			}
		}
	}

	var hub *realtime.Hub
	if db != nil {
		hub = realtime.NewHub(realtime.Options{})
		defer hub.Close()
		relay := notifications.Relay(db, hub)
		go realtime.Listen(signals, configuration.DatabaseURL, notifications.Channel, func(ctx context.Context, payload string) {
			relay(ctx, payload)
			teamsService.Notify(payload)
		}, hub.Resync)
		// Followed persons and clusters: what changed becomes notifications (follows package).
		go follows.Run(signals, db, configuration.DatabaseURL)
	}

	deps := api.Dependencies{
		DB:          db,
		JWTSecret:   configuration.JWTSecret,
		EmailDomain: configuration.EmailDomain,
		Storage:     store,
		Queue:       queueClient,
		Hub:         hub,
		Teams:       teamsService,
		FaceVision:  faceVision,
		Geocoder:    geocoder,
		Fingerprint: fingerprintClient,
		// The report editor and the PDF renderer are WordCraft's (docs/facial-reports-plan.md).
		ReportRenderer:  reportRenderer(),
		WordcraftWebDir: os.Getenv("WORDCRAFT_WEB_DIR"),
		AssistDrafter:   assistDrafter,
		AssistModel:     configuration.AssistModel,
		AskModel:        askModel,
		VideoLab:        videoLab,
		TrustedProxies:  configuration.TrustedProxies,
		ClientConfig:    api.ClientConfig{MapTileURL: configuration.MapTileURL, MapTileAttribution: configuration.MapTileAttribution},
	}
	if options.Register != nil {
		deps.Register = func(engine *gin.Engine, protected *gin.RouterGroup) {
			options.Register(deps, engine, protected)
		}
	}
	router := api.NewRouter(deps)

	if distDir := web.DistDir(); distDir != "" {
		log.Printf("serving prototype frontend from %s", distDir)
		router.NoRoute(gin.WrapH(web.SPAHandler(distDir)))
	} else {
		log.Printf("frontend build not found (build it with: cd frontend && npm run build); serving API only")
	}

	log.Printf("trackid backend listening on :%s", configuration.Port)
	httpServer := &http.Server{Addr: ":" + configuration.Port, Handler: router.Handler()}
	serveErr := make(chan error, 1)
	go func() { serveErr <- httpServer.ListenAndServe() }()
	select {
	case err := <-serveErr:
		return err
	case <-signals.Done():
	}

	// Shutdown does not wait for WebSockets, which it can't see once upgraded, so close them first.
	if hub != nil {
		hub.Close()
	}
	shutdown, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := httpServer.Shutdown(shutdown); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// shutdownTimeout is how long requests in progress get to finish when the server is stopped.
const shutdownTimeout = 10 * time.Second

// reportRenderer renders issued reports with wordcraft-cli at WORDCRAFT_CLI; nil when unset.
func reportRenderer() reports.Renderer {
	path := os.Getenv("WORDCRAFT_CLI")
	if path == "" {
		log.Printf("WORDCRAFT_CLI not set: reports can be drafted but not issued")
		return nil
	}
	return reports.CLIRenderer{Path: path}
}
