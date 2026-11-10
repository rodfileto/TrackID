package config

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"
)

type Config struct {
	// Environment is TRACKID_ENV: "development" (the default) or "production". In production a
	// setting the deployment can't do without, or an integration it configured that doesn't come
	// up, stops the process (CheckProduction, Unavailable) instead of leaving a feature off.
	Environment string
	// TrustedProxies are the addresses or CIDR ranges (TRUSTED_PROXIES, comma-separated) whose
	// X-Forwarded-For the API believes for a client's address. Empty trusts none.
	TrustedProxies []string

	Port        string
	DatabaseURL string
	RedisURL    string
	JWTSecret   string
	EmailDomain string
	S3Endpoint  string
	S3AccessKey string
	S3SecretKey string
	S3Bucket    string

	// Vision: face detection on evidence in the API (cases.DetectFaces) and
	// embedding in the worker (embedding.ComputeForCodification). Left empty,
	// both log a warning and run without it rather than failing to start.
	VisionDetector          string // "yunet" (default) or "scrfd"; see README "Models and licences"
	VisionDetectorPath      string
	VisionRecognizerPath    string
	VisionSharedLibraryPath string
	VisionUseGPU            bool

	// FingerprintSidecarURL is the sourceafis-sidecar service the worker
	// extracts and matches fingerprint templates with (see the fingerprint
	// package). Left empty, fingerprint tasks aren't processed.
	FingerprintSidecarURL string

	// Geocoder: Nominatim (OpenStreetMap) for locations -- the address and
	// coordinate picker, and importers with only an address (geocode package).
	// GeocoderURL defaults to the public instance, whose usage policy forbids
	// autocomplete: search-as-you-type needs a self-hosted one.
	GeocoderURL         string
	GeocoderUserAgent   string
	GeocoderEmail       string
	GeocoderMinInterval time.Duration // 0: the geocode package's default (1s on the public instance)

	// Assist: a local language model that drafts a registration from a request's files (assist
	// package). AssistEndpoint is the OpenAI-compatible base URL (e.g. "http://127.0.0.1:8080/v1")
	// of a local server such as Strata or Ollama; AssistModel names the model. Both empty means
	// drafting is disabled and the worker logs a warning rather than failing to start.
	AssistEndpoint string
	AssistModel    string
	// AssistAPIKey is sent as a Bearer token when the local server requires one (Strata does);
	// empty sends no Authorization header (Ollama needs none).
	AssistAPIKey string
	// AssistTimeout bounds one draft (ASSIST_TIMEOUT, a Go duration such as "5m"); zero means
	// assist.DefaultTimeout.
	AssistTimeout time.Duration

	// Ask: the AI mode's model (ask package), anything speaking the OpenAI chat protocol with tool
	// calling, local or not. AskEndpoint and AskModel empty leave the AI mode off, unless the assist
	// model is configured, which it then shares. An API key never crosses to another endpoint: the
	// assist settings are taken together or not at all.
	AskEndpoint string
	AskModel    string
	AskAPIKey   string
	// AskExtraBody is a JSON object of request fields the server wants (ASK_EXTRA_BODY).
	AskExtraBody string
	// AskTimeout bounds one question (ASK_TIMEOUT); zero means ask.DefaultTimeout.
	AskTimeout time.Duration

	// Teams: each user may set a Microsoft Teams webhook and be told of their notifications there (teams package,
	// docs/follow-plan.md). SecretsKey encrypts the webhooks (SECRETS_KEY, 32 bytes in base64 or hex); without it
	// Teams is off. AppURL is the frontend's base URL, which the message's button opens (APP_URL).
	// TeamsWebhookHosts are the host suffixes a webhook may point at (TEAMS_WEBHOOK_HOSTS, comma-separated; empty
	// means the teams package's default). TeamsLanguage is the messages' language (TEAMS_LANGUAGE: pt-BR or en).
	SecretsKey        string
	AppURL            string
	TeamsWebhookHosts string
	TeamsLanguage     string

	// The frontend's base map, served to it at runtime by /api/config (MAP_TILE_URL, a raster tile URL
	// template, and MAP_TILE_ATTRIBUTION, its credit as HTML). Empty means OpenStreetMap's public tiles.
	MapTileURL         string
	MapTileAttribution string
}

func Load() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
	}
	detector := os.Getenv("VISION_DETECTOR")
	if detector == "" {
		detector = "yunet"
	}
	userAgent := os.Getenv("GEOCODER_USER_AGENT")
	if userAgent == "" {
		userAgent = "trackid (+https://github.com/rodfileto/trackid)"
	}
	var minInterval time.Duration
	if v := os.Getenv("GEOCODER_MIN_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			log.Fatalf("GEOCODER_MIN_INTERVAL: %v", err)
		}
		minInterval = d
	}
	var assistTimeout time.Duration
	if v := os.Getenv("ASSIST_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			log.Fatalf("ASSIST_TIMEOUT: %v", err)
		}
		assistTimeout = d
	}
	var askTimeout time.Duration
	if v := os.Getenv("ASK_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			log.Fatalf("ASK_TIMEOUT: %v", err)
		}
		askTimeout = d
	}
	askEndpoint, askModel, askAPIKey := os.Getenv("ASK_ENDPOINT"), os.Getenv("ASK_MODEL"), os.Getenv("ASK_API_KEY")
	if askEndpoint == "" && askModel == "" {
		askEndpoint, askModel, askAPIKey = os.Getenv("ASSIST_ENDPOINT"), os.Getenv("ASSIST_MODEL"), os.Getenv("ASSIST_API_KEY")
	}

	environment := os.Getenv("TRACKID_ENV")
	switch environment {
	case "":
		environment = EnvironmentDevelopment
	case EnvironmentDevelopment, EnvironmentProduction:
	default:
		log.Fatalf("TRACKID_ENV must be %q or %q, got %q", EnvironmentDevelopment, EnvironmentProduction, environment)
	}
	trustedProxies, err := parseTrustedProxies(os.Getenv("TRUSTED_PROXIES"))
	if err != nil {
		log.Fatalf("TRUSTED_PROXIES: %v", err)
	}

	return Config{
		Environment:    environment,
		TrustedProxies: trustedProxies,

		Port:        port,
		DatabaseURL: os.Getenv("DATABASE_URL"),
		RedisURL:    os.Getenv("REDIS_URL"),
		JWTSecret:   os.Getenv("JWT_SECRET"),
		EmailDomain: os.Getenv("EMAIL_DOMAIN"),
		S3Endpoint:  os.Getenv("S3_ENDPOINT"),
		S3AccessKey: os.Getenv("S3_ACCESS_KEY"),
		S3SecretKey: os.Getenv("S3_SECRET_KEY"),
		S3Bucket:    os.Getenv("S3_BUCKET"),

		SecretsKey:        os.Getenv("SECRETS_KEY"),
		AppURL:            os.Getenv("APP_URL"),
		TeamsWebhookHosts: os.Getenv("TEAMS_WEBHOOK_HOSTS"),
		TeamsLanguage:     os.Getenv("TEAMS_LANGUAGE"),

		MapTileURL:         os.Getenv("MAP_TILE_URL"),
		MapTileAttribution: os.Getenv("MAP_TILE_ATTRIBUTION"),

		VisionDetector:          detector,
		VisionDetectorPath:      os.Getenv("VISION_DETECTOR_PATH"),
		VisionRecognizerPath:    os.Getenv("VISION_RECOGNIZER_PATH"),
		VisionSharedLibraryPath: os.Getenv("VISION_SHARED_LIBRARY_PATH"),
		VisionUseGPU:            os.Getenv("VISION_USE_GPU") == "true",

		FingerprintSidecarURL: os.Getenv("FINGERPRINT_SIDECAR_URL"),

		GeocoderURL:         os.Getenv("GEOCODER_URL"),
		GeocoderUserAgent:   userAgent,
		GeocoderEmail:       os.Getenv("GEOCODER_EMAIL"),
		GeocoderMinInterval: minInterval,

		AssistEndpoint: os.Getenv("ASSIST_ENDPOINT"),
		AssistModel:    os.Getenv("ASSIST_MODEL"),
		AssistAPIKey:   os.Getenv("ASSIST_API_KEY"),
		AssistTimeout:  assistTimeout,

		AskEndpoint:  askEndpoint,
		AskModel:     askModel,
		AskAPIKey:    askAPIKey,
		AskExtraBody: os.Getenv("ASK_EXTRA_BODY"),
		AskTimeout:   askTimeout,
	}
}

// The values of TRACKID_ENV.
const (
	EnvironmentDevelopment = "development"
	EnvironmentProduction  = "production"
)

// MinJWTSecretBytes is the shortest JWT_SECRET production accepts: 32 bytes, an HS256 key's size.
const MinJWTSecretBytes = 32

// Production reports whether TRACKID_ENV is production.
func (c Config) Production() bool { return c.Environment == EnvironmentProduction }

// Process names what CheckProduction checks the settings for.
type Process int

const (
	ProcessAPI Process = iota
	ProcessWorker
)

// CheckProduction returns what is missing or unfit, all of it, for process to run in production:
// the database, the queue and object storage, and for the API a JWT_SECRET of at least
// MinJWTSecretBytes and APP_URL over https. Nil outside production, where a missing setting only
// turns its feature off.
func (c Config) CheckProduction(process Process) error {
	if !c.Production() {
		return nil
	}
	var problems []error
	require := func(name, value string) {
		if value == "" {
			problems = append(problems, fmt.Errorf("%s is not set", name))
		}
	}
	require("DATABASE_URL", c.DatabaseURL)
	require("REDIS_URL", c.RedisURL)
	require("S3_ENDPOINT", c.S3Endpoint)
	require("S3_ACCESS_KEY", c.S3AccessKey)
	require("S3_SECRET_KEY", c.S3SecretKey)
	require("S3_BUCKET", c.S3Bucket)
	if process == ProcessAPI {
		if len(c.JWTSecret) < MinJWTSecretBytes {
			problems = append(problems, fmt.Errorf("JWT_SECRET must be at least %d bytes (openssl rand -hex 32)", MinJWTSecretBytes))
		}
		if !strings.HasPrefix(c.AppURL, "https://") {
			problems = append(problems, errors.New("APP_URL must be the https:// address users open"))
		}
	}
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("TRACKID_ENV=production: %w", errors.Join(problems...))
}

// Unavailable reports that an integration the deployment configured did not come up. Outside
// production it logs and the process goes on without it, as development expects; in production it
// stops the process, since a feature someone set up would otherwise be off without anyone noticing.
func (c Config) Unavailable(format string, args ...any) {
	if c.Production() {
		log.Fatalf("TRACKID_ENV=production, stopping: "+format, args...)
	}
	log.Printf(format, args...)
}

// parseTrustedProxies splits a comma-separated list of addresses and CIDR ranges, checking each.
func parseTrustedProxies(value string) ([]string, error) {
	var proxies []string
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, _, err := net.ParseCIDR(item); err != nil && net.ParseIP(item) == nil {
			return nil, fmt.Errorf("%q is neither an address nor a CIDR range", item)
		}
		proxies = append(proxies, item)
	}
	return proxies, nil
}
