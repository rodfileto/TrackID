package api

import (
	"database/sql"

	"github.com/gin-gonic/gin"

	"github.com/rodfileto/trackid/auth"
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
	api.POST("/auth/register", RegisterHandler(authRepository, deps.EmailDomain))
	api.POST("/auth/login", LoginHandler(authRepository, deps.JWTSecret))

	protected := api.Group("/")
	protected.Use(Middleware(deps.JWTSecret))
	protected.GET("me", MeHandler(authRepository))
	protected.GET("cases", ListCasesHandler(deps.DB))
	protected.POST("cases", CreateCaseHandler(deps.DB))
	protected.GET("cases/:caseId", GetCaseHandler(deps.DB))
	protected.POST("cases/:caseId/evidences", AddEvidenceHandler(deps.DB, deps.Storage))
	protected.DELETE("cases/:caseId/evidences/:evidenceId", DeleteEvidenceHandler(deps.DB))
	protected.GET("cases/:caseId/evidences/:evidenceId/download", DownloadEvidenceHandler(deps.DB, deps.Storage))
	protected.GET("cases/:caseId/evidences/:evidenceId/traces", ListTracesHandler(deps.DB))
	protected.POST("cases/:caseId/evidences/:evidenceId/traces", CreateTracesHandler(deps.DB))
	protected.DELETE("cases/:caseId/evidences/:evidenceId/traces/:traceId", DeleteTraceHandler(deps.DB))
	protected.GET("cases/:caseId/evidences/:evidenceId/traces/:traceId/points", ListPointsHandler(deps.DB))
	protected.POST("cases/:caseId/evidences/:evidenceId/traces/:traceId/points", AddPointsHandler(deps.DB))
	protected.PUT("cases/:caseId/evidences/:evidenceId/traces/:traceId/points/:pointId", UpdatePointHandler(deps.DB))
	protected.DELETE("cases/:caseId/evidences/:evidenceId/traces/:traceId/points/:pointId", DeletePointHandler(deps.DB))
	protected.POST("cases/:caseId/evidences/:evidenceId/traces/:traceId/codification-image", SaveCodificationImageHandler(deps.DB, deps.Storage))

	if deps.Register != nil {
		deps.Register(router, protected)
	}
	return router
}
