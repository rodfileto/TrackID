package api

import (
	"database/sql"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"

	"github.com/rodfileto/trackid/auth"
	"github.com/rodfileto/trackid/cases"
	"github.com/rodfileto/trackid/geocode"
	"github.com/rodfileto/trackid/storage"
)

// Dependencies are the integrations NewRouter wires into the API.
// Any field may be nil/zero when its integration isn't configured; every
// handler that depends on one already treats a nil dependency as "not
// configured" (503) rather than panicking.
type Dependencies struct {
	DB          *sql.DB
	JWTSecret   string
	EmailDomain string
	Storage     *storage.Client
	// Queue enqueues background jobs (see the embedding package). A nil Queue
	// means the request path that would enqueue one just logs and moves on --
	// see cases.SaveCodificationImage.
	Queue *asynq.Client
	// FaceVision backs face detection on evidence images and machine face
	// comparison. Nil means detect-faces returns 503, and comparisons only
	// work between codifications that already have stored embeddings.
	FaceVision cases.FaceVision
	// Geocoder backs the occurrence form's address search and map-click addresses. Nil means
	// those endpoints return 503; occurrences can still be placed on the map.
	Geocoder geocode.Geocoder
	// Register is an optional hook called after the core routes are registered.
	// It receives the engine and the authenticated route group (mounted under
	// /api) so integrations can attach their own routes. May be nil.
	Register func(engine *gin.Engine, protected *gin.RouterGroup)
}

func NewRouter(deps Dependencies) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())
	router.GET("/health", healthHandler)
	router.GET("/health/db", DatabaseHealthHandler(deps.DB))

	api := router.Group("/api")
	authRepository := auth.NewRepository(deps.DB)
	// There is no public sign-up: the first SUPER_ADMIN comes from cmd/create-superuser and
	// admins create every other account.
	api.POST("/auth/login", LoginHandler(authRepository, deps.JWTSecret))

	protected := api.Group("/")
	protected.Use(Middleware(deps.JWTSecret), ActiveUser(authRepository))
	protected.GET("me", MeHandler())
	protected.GET("me/analyses", MyAnalysesHandler(deps.DB))

	admin := protected.Group("admin")
	admin.Use(RequireAdmin())
	admin.GET("users", ListUsersHandler(authRepository))
	admin.POST("users", CreateUserHandler(authRepository, deps.EmailDomain))
	admin.PATCH("users/:userId", UpdateUserAccessHandler(authRepository))
	admin.GET("distribution", DistributionStatusHandler(deps.DB))
	admin.POST("distribution", DistributeHandler(deps.DB))
	admin.POST("cases/:caseId/analyses", OpenAnalysisHandler(deps.DB))
	admin.PUT("analyses/:analysisId/assignment", AssignAnalysisHandler(deps.DB))
	protected.GET("cases", ListCasesHandler(deps.DB))
	protected.POST("cases", CreateCaseHandler(deps.DB))
	protected.GET("cases/years", ListCaseYearsHandler(deps.DB))

	// Everything about one case: admins see every case, anyone else only the cases where they hold
	// an analysis.
	oneCase := protected.Group("cases/:caseId")
	oneCase.Use(CaseAccess(deps.DB))
	oneCase.GET("", GetCaseHandler(deps.DB))
	oneCase.GET("codifications", ListCaseCodificationsHandler(deps.DB))
	oneCase.GET("clusters", ListCaseClustersHandler(deps.DB))
	oneCase.GET("identity-decisions", ListCaseIdentityDecisionsHandler(deps.DB))
	oneCase.GET("analyses", ListCaseAnalysesHandler(deps.DB))
	oneCase.GET("analyses/:analysisId/assignments", AnalysisAssignmentsHandler(deps.DB))
	oneCase.POST("analyses/:analysisId/report", IssueReportHandler(deps.DB))
	oneCase.GET("analyses/:analysisId/pairs", AnalysisWorksheetHandler(deps.DB))
	oneCase.GET("analyses/:analysisId/pairs/image", PairImageHandler(deps.DB, deps.Storage))
	oneCase.PUT("analyses/:analysisId/decisions", SaveDraftHandler(deps.DB))
	oneCase.POST("analyses/:analysisId/submit", SubmitStepHandler(deps.DB, deps.Queue))
	oneCase.POST("evidences", AddEvidenceHandler(deps.DB, deps.Storage))
	oneCase.DELETE("evidences/:evidenceId", DeleteEvidenceHandler(deps.DB))
	oneCase.GET("evidences/:evidenceId/download", DownloadEvidenceHandler(deps.DB, deps.Storage))
	oneCase.POST("codifications/compare", CompareFacesHandler(deps.DB, deps.Storage, deps.FaceVision))
	oneCase.POST("evidences/:evidenceId/detect-faces", DetectFacesHandler(deps.DB, deps.Storage, deps.FaceVision))
	oneCase.GET("evidences/:evidenceId/traces", ListTracesHandler(deps.DB))
	oneCase.POST("evidences/:evidenceId/traces", CreateTracesHandler(deps.DB, deps.Queue))
	oneCase.DELETE("evidences/:evidenceId/traces/:traceId", DeleteTraceHandler(deps.DB))
	oneCase.GET("evidences/:evidenceId/traces/:traceId/points", ListPointsHandler(deps.DB))
	oneCase.POST("evidences/:evidenceId/traces/:traceId/points", AddPointsHandler(deps.DB))
	oneCase.PUT("evidences/:evidenceId/traces/:traceId/points/:pointId", UpdatePointHandler(deps.DB))
	oneCase.DELETE("evidences/:evidenceId/traces/:traceId/points/:pointId", DeletePointHandler(deps.DB))
	oneCase.POST("evidences/:evidenceId/traces/:traceId/codification-image", SaveCodificationImageHandler(deps.DB, deps.Storage, deps.Queue))

	protected.GET("persons/search", SearchPersonsHandler(deps.DB))
	protected.POST("persons/search-by-face", SearchPersonsByFaceHandler(deps.DB, deps.FaceVision))
	protected.GET("persons/:personId", GetPersonProfileHandler(deps.DB))
	protected.GET("persons/:personId/identity", GetPersonIdentityHandler(deps.DB))
	protected.GET("persons/:personId/identity-files/:identityFileId/download", DownloadIdentityFileHandler(deps.DB, deps.Storage))
	protected.GET("persons/:personId/clusters", ListPersonClustersHandler(deps.DB))
	protected.GET("persons/:personId/cases", ListPersonCasesHandler(deps.DB))

	protected.GET("clusters", ListClustersHandler(deps.DB))
	protected.GET("clusters/graph", ClustersGraphHandler(deps.DB))

	protected.GET("occurrences/map", OccurrenceMapHandler(deps.DB))
	protected.GET("occurrences/types", OccurrenceTypesHandler(deps.DB))
	protected.GET("occurrences/years", OccurrenceYearsHandler(deps.DB))
	protected.GET("occurrences/search", SearchOccurrencesHandler(deps.DB))
	protected.POST("occurrences", CreateOccurrenceHandler(deps.DB))
	oneCase.GET("occurrence", CaseOccurrenceHandler(deps.DB))
	oneCase.PUT("occurrence", LinkCaseOccurrenceHandler(deps.DB))
	protected.GET("locations/nearby", NearbyLocationsHandler(deps.DB))
	protected.GET("geocode/search", GeocodeSearchHandler(deps.Geocoder))
	protected.GET("geocode/reverse", GeocodeReverseHandler(deps.Geocoder))

	if deps.Register != nil {
		deps.Register(router, protected)
	}
	return router
}
