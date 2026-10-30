package api

import (
	"database/sql"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"

	"github.com/rodfileto/trackid/auth"
	"github.com/rodfileto/trackid/cases"
	"github.com/rodfileto/trackid/fingerprint"
	"github.com/rodfileto/trackid/geocode"
	"github.com/rodfileto/trackid/reports"
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
	// Fingerprint scores the image lab's renders with SourceAFIS. Nil means the lab renders but
	// does not compare.
	Fingerprint *fingerprint.Client
	// Geocoder backs the occurrence form's address search and map-click addresses. Nil means
	// those endpoints return 503; occurrences can still be placed on the map.
	Geocoder geocode.Geocoder
	// ReportRenderer renders an issued report's PDF (reports.CLIRenderer over WORDCRAFT_CLI). Nil
	// means issuing returns 503; drafts still work.
	ReportRenderer reports.Renderer
	// WordcraftWebDir is WordCraft's web build, served at /wordcraft/ so the report editor runs on
	// the app's own origin (its postMessage bridge only talks to a same-origin parent). Empty
	// serves nothing there.
	WordcraftWebDir string
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
	if deps.WordcraftWebDir != "" {
		router.Static("/wordcraft", deps.WordcraftWebDir)
	}

	api := router.Group("/api")
	authRepository := auth.NewRepository(deps.DB)
	// There is no public sign-up: the first SUPER_ADMIN comes from cmd/create-superuser and
	// admins create every other account.
	api.POST("/auth/login", LoginHandler(authRepository, deps.JWTSecret))

	protected := api.Group("/")
	protected.Use(Middleware(deps.JWTSecret), ActiveUser(authRepository))
	protected.GET("me", MeHandler())
	protected.PATCH("me", UpdateMeHandler(authRepository))
	protected.PUT("me/password", ChangeMyPasswordHandler(authRepository))
	protected.PUT("me/avatar", SetMyAvatarHandler(authRepository))
	protected.DELETE("me/avatar", RemoveMyAvatarHandler(authRepository))
	protected.GET("users/:userId/avatar", UserAvatarHandler(authRepository))
	protected.GET("me/analyses", MyAnalysesHandler(deps.DB))
	// The home page's illustration, anonymous: every signed-in user sees it.
	protected.GET("overview/graph", OverviewGraphHandler(deps.DB))

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
	coordination.POST("cases/:caseId/analyses/:analysisId/pairs", AddPairsHandler(deps.DB))
	coordination.PUT("analyses/:analysisId/assignment", AssignAnalysisHandler(deps.DB))
	coordination.GET("analyses/:analysisId/prior-deciders", PriorDecidersHandler(deps.DB))
	coordination.GET("requests", ListRequestsHandler(deps.DB))
	coordination.POST("requests", RegisterRequestHandler(deps.DB))
	coordination.POST("cases/:caseId/requests", CreateCaseRequestHandler(deps.DB))
	coordination.POST("cases/:caseId/requests/:requestId/files", AttachRequestFilesHandler(deps.DB))
	coordination.POST("cases/:caseId/requests/:requestId/material", ReceiveMaterialHandler(deps.DB, deps.Storage, deps.Queue))
	// The image lab: fingerprint processing tried on any image, nothing stored
	// (docs/fingerprint-image-processing-plan.md). Every signed-in user may use it.
	lab := NewImageLab(deps.Fingerprint)
	protected.GET("imageproc", lab.InfoHandler())
	protected.POST("imageproc/sources", lab.UploadHandler())
	protected.POST("imageproc/estimate", lab.EstimateHandler())
	protected.POST("imageproc/render", lab.RenderHandler())
	protected.POST("imageproc/compare", lab.CompareHandler())
	// The face lab: a one-to-one facial comparison after FISWG on any two images, nothing stored
	// (docs/facial-comparison-tool-plan.md).
	faceLab := NewFaceLab(deps.FaceVision)
	protected.GET("facelab", faceLab.FaceInfoHandler())
	protected.POST("facelab/sources", faceLab.FaceUploadHandler())
	protected.GET("facelab/sources/:id", faceLab.FaceSourceHandler())
	protected.POST("facelab/render", faceLab.FaceRenderHandler())
	protected.POST("facelab/quality", faceLab.FaceQualityHandler())
	protected.POST("facelab/align", faceLab.FaceAlignHandler())
	protected.POST("facelab/check", faceLab.FaceCheckHandler())
	protected.POST("facelab/sheet", faceLab.FaceSheetHandler())
	protected.GET("cases", ListCasesHandler(deps.DB))
	protected.POST("cases", CreateCaseHandler(deps.DB))
	protected.GET("cases/years", ListCaseYearsHandler(deps.DB))

	// Everything about one case: analysts and coordinators read every case, anyone else only the cases
	// where they hold an analysis (distribution.Rights).
	oneCase := protected.Group("cases/:caseId")
	oneCase.Use(CaseAccess(deps.DB))
	oneCase.GET("", GetCaseHandler(deps.DB))
	oneCase.GET("codifications", ListCaseCodificationsHandler(deps.DB))
	oneCase.GET("clusters", ListCaseClustersHandler(deps.DB))
	oneCase.GET("identity-decisions", ListCaseIdentityDecisionsHandler(deps.DB))
	oneCase.GET("analyses", ListCaseAnalysesHandler(deps.DB))
	oneCase.GET("requests", ListCaseRequestsHandler(deps.DB))
	oneCase.GET("material", ListCaseMaterialHandler(deps.DB))
	oneCase.GET("videos", ListVideosHandler(deps.DB))
	oneCase.POST("videos", AddVideoHandler(deps.DB, deps.Storage, deps.Queue))
	oneCase.POST("videos/:fileId/frames", ExtractFrameHandler(deps.DB, deps.Storage, deps.Queue))
	oneCase.PUT("traces/:traceId/role", SetTraceRoleHandler(deps.DB))
	oneCase.GET("analyses/:analysisId/assignments", AnalysisAssignmentsHandler(deps.DB))
	oneCase.GET("analyses/:analysisId/report", GetReportHandler(deps.DB))
	oneCase.GET("analyses/:analysisId/report/draft", ReportDraftHandler(deps.DB, deps.Storage))
	oneCase.PUT("analyses/:analysisId/report/draft", SaveReportDraftHandler(deps.DB, deps.Storage))
	oneCase.POST("analyses/:analysisId/report/draft/refresh", RefreshReportDraftHandler(deps.DB, deps.Storage))
	oneCase.POST("analyses/:analysisId/report", IssueReportHandler(deps.DB, deps.Storage, deps.ReportRenderer))
	oneCase.GET("analyses/:analysisId/report/document", ReportDocumentHandler(deps.DB, deps.Storage))
	oneCase.POST("analyses/:analysisId/report/deliveries", DeliverReportHandler(deps.DB))
	oneCase.GET("analyses/:analysisId/pairs", AnalysisWorksheetHandler(deps.DB))
	oneCase.PUT("analyses/:analysisId/suitability", AssessTraceHandler(deps.DB))
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

	// What the occurrence form needs: a coordinator registering a request creates the occurrence
	// it cites (docs/facial-reports-plan.md, "Request first"); the map and the rest stay
	// intelligence's.
	occurrenceForm := protected.Group("")
	occurrenceForm.Use(RequireOccurrenceForm(deps.DB))
	intelligence.GET("occurrences/map", OccurrenceMapHandler(deps.DB))
	occurrenceForm.GET("occurrences/types", OccurrenceTypesHandler(deps.DB))
	intelligence.GET("occurrences/years", OccurrenceYearsHandler(deps.DB))
	occurrenceForm.GET("occurrences/search", SearchOccurrencesHandler(deps.DB))
	intelligence.POST("occurrences", CreateOccurrenceHandler(deps.DB))
	oneCase.GET("occurrence", CaseOccurrenceHandler(deps.DB))
	oneCase.PUT("occurrence", RequireIntelligence(deps.DB), LinkCaseOccurrenceHandler(deps.DB))
	occurrenceForm.GET("locations/nearby", NearbyLocationsHandler(deps.DB))
	occurrenceForm.GET("geocode/search", GeocodeSearchHandler(deps.Geocoder))
	occurrenceForm.GET("geocode/reverse", GeocodeReverseHandler(deps.Geocoder))

	if deps.Register != nil {
		deps.Register(router, protected)
	}
	return router
}
