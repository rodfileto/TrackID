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

	// Account management only (docs/access-profiles-plan.md): an admin sees no case or person
	// content unless they also hold a capability.
	admin := protected.Group("admin")
	admin.Use(RequireAdmin())
	admin.GET("users", ListUsersHandler(authRepository))
	admin.POST("users", CreateUserHandler(authRepository, deps.EmailDomain))
	admin.PATCH("users/:userId", UpdateUserAccessHandler(authRepository))
	admin.PUT("users/:userId/password", SetUserPasswordHandler(authRepository))
	// Coordination: handing out the work and opening comparisons. The paths keep their old
	// admin/ prefix; the guard is the coordinator capability.
	coordination := protected.Group("admin")
	coordination.Use(RequireCoordinator(deps.DB))
	coordination.GET("distribution", DistributionStatusHandler(deps.DB))
	coordination.POST("distribution", DistributeHandler(deps.DB))
	coordination.POST("cases/:caseId/analyses", OpenAnalysisHandler(deps.DB))
	coordination.PUT("analyses/:analysisId/assignment", AssignAnalysisHandler(deps.DB))
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

	// A person's identity is for examiners and analysts; the rest of the person page, and every
	// intelligence view, for analysts only.
	identity := protected.Group("persons")
	identity.Use(RequirePersonIdentity(deps.DB))
	identity.GET("search", SearchPersonsHandler(deps.DB))
	identity.GET(":personId/identity", GetPersonIdentityHandler(deps.DB))
	identity.GET(":personId/identity-files/:identityFileId/download", DownloadIdentityFileHandler(deps.DB, deps.Storage))

	intelligence := protected.Group("")
	intelligence.Use(RequireIntelligence(deps.DB))
	// Each read of a person's history is logged (access_log), as are case reads by someone who
	// holds none of the case's analyses (CaseAccess).
	intelligence.POST("persons/search-by-face", LogAccess(deps.DB, accessFaceSearch, ""), SearchPersonsByFaceHandler(deps.DB, deps.FaceVision))
	intelligence.GET("persons/:personId", LogAccess(deps.DB, accessPerson, "personId"), GetPersonProfileHandler(deps.DB))
	intelligence.GET("persons/:personId/clusters", LogAccess(deps.DB, accessPerson, "personId"), ListPersonClustersHandler(deps.DB))
	intelligence.GET("persons/:personId/cases", LogAccess(deps.DB, accessPerson, "personId"), ListPersonCasesHandler(deps.DB))

	intelligence.GET("clusters", ListClustersHandler(deps.DB))
	intelligence.GET("clusters/graph", ClustersGraphHandler(deps.DB))

	intelligence.GET("occurrences/map", OccurrenceMapHandler(deps.DB))
	intelligence.GET("occurrences/types", OccurrenceTypesHandler(deps.DB))
	intelligence.GET("occurrences/years", OccurrenceYearsHandler(deps.DB))
	intelligence.GET("occurrences/search", SearchOccurrencesHandler(deps.DB))
	intelligence.POST("occurrences", CreateOccurrenceHandler(deps.DB))
	oneCase.GET("occurrence", CaseOccurrenceHandler(deps.DB))
	oneCase.PUT("occurrence", RequireIntelligence(deps.DB), LinkCaseOccurrenceHandler(deps.DB))
	intelligence.GET("locations/nearby", NearbyLocationsHandler(deps.DB))
	intelligence.GET("geocode/search", GeocodeSearchHandler(deps.Geocoder))
	intelligence.GET("geocode/reverse", GeocodeReverseHandler(deps.Geocoder))

	if deps.Register != nil {
		deps.Register(router, protected)
	}
	return router
}
