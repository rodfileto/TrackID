package api

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rodrigorfcm/trackid/backend/internal/auth"
	"github.com/rodrigorfcm/trackid/backend/internal/database"
	"github.com/rodrigorfcm/trackid/backend/internal/fingerprintcase"
	"github.com/rodrigorfcm/trackid/backend/internal/infobio"
)

// Dependencies are the optional integrations NewRouter wires into the API.
// Any field may be nil/zero when its integration isn't configured; every
// handler that depends on one already treats a nil dependency as "not
// configured" (503) rather than panicking.
type Dependencies struct {
	DB                     *sql.DB
	JWTSecret              string
	InfoBioService         *infobio.SearchService
	InfoBioDownloadService *infobio.DownloadService
	InfoBioSessionManager  *infobio.SessionManager
}

func NewRouter(deps Dependencies) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())
	router.GET("/health", healthHandler)
	router.GET("/health/db", database.HealthHandler(deps.DB))
	authRepository := auth.NewRepository(deps.DB)
	router.POST("/auth/register", auth.RegisterHandler(authRepository))
	router.POST("/auth/login", auth.LoginHandler(authRepository, deps.JWTSecret))

	protected := router.Group("/")
	protected.Use(auth.Middleware(deps.JWTSecret))
	protected.GET("me", auth.MeHandler(authRepository))
	protected.GET("graph/infobio/search", gin.WrapH(infoBioHandler(deps.InfoBioService)))
	protected.GET("graph/infobio/nifs/:nif/image", infoBioDownloadHandler(deps.InfoBioDownloadService, func(service *infobio.DownloadService) gin.HandlerFunc { return service.JPGHandler() }))
	protected.GET("graph/infobio/nifs/:nif/pdf", infoBioDownloadHandler(deps.InfoBioDownloadService, func(service *infobio.DownloadService) gin.HandlerFunc { return service.PDFHandler() }))
	protected.GET("graph/infobio/nifs/:nif/nist", infoBioDownloadHandler(deps.InfoBioDownloadService, func(service *infobio.DownloadService) gin.HandlerFunc { return service.NISTHandler() }))
	protected.GET("infobio/auth", infobio.AuthStatusHandler(deps.InfoBioSessionManager))
	protected.POST("infobio/auth", infobio.AuthCreateHandler(deps.InfoBioSessionManager))
	protected.DELETE("infobio/auth", infobio.AuthDeleteHandler(deps.InfoBioSessionManager))
	protected.POST("infobio/search", infobio.SearchHandler(deps.InfoBioService, deps.InfoBioSessionManager))
	protected.POST("infobio/details", infobio.DetailsHandler(deps.InfoBioService, deps.InfoBioSessionManager))
	protected.GET("toolkit/fingerprint-cases", fingerprintcase.ListHandler(deps.DB))
	return router
}

func infoBioDownloadHandler(service *infobio.DownloadService, handler func(*infobio.DownloadService) gin.HandlerFunc) gin.HandlerFunc {
	if service == nil {
		return func(context *gin.Context) {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "infobio downloads are not configured"})
		}
	}
	return handler(service)
}

func infoBioHandler(service *infobio.SearchService) http.Handler {
	if service == nil {
		return http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
			http.Error(responseWriter, "infobio search is not configured", http.StatusServiceUnavailable)
		})
	}
	return service.SearchHandler()
}

func healthHandler(context *gin.Context) {
	context.JSON(http.StatusOK, gin.H{"status": "ok"})
}
