package api

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rodrigorfcm/trackid/auth"
	"github.com/rodrigorfcm/trackid/cases"
	"github.com/rodrigorfcm/trackid/database"
)

// Dependencies are the integrations NewRouter wires into the API.
// Any field may be nil/zero when its integration isn't configured; every
// handler that depends on one already treats a nil dependency as "not
// configured" (503) rather than panicking.
type Dependencies struct {
	DB          *sql.DB
	JWTSecret   string
	EmailDomain string
	// Register is an optional hook called after the core routes are registered.
	// It receives the engine and the authenticated route group so integrations
	// can attach their own routes. May be nil.
	Register func(engine *gin.Engine, protected *gin.RouterGroup)
}

func NewRouter(deps Dependencies) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())
	router.GET("/health", healthHandler)
	router.GET("/health/db", database.HealthHandler(deps.DB))
	authRepository := auth.NewRepository(deps.DB)
	router.POST("/auth/register", auth.RegisterHandler(authRepository, deps.EmailDomain))
	router.POST("/auth/login", auth.LoginHandler(authRepository, deps.JWTSecret))

	protected := router.Group("/")
	protected.Use(auth.Middleware(deps.JWTSecret))
	protected.GET("me", auth.MeHandler(authRepository))
	protected.GET("cases", cases.ListHandler(deps.DB))

	if deps.Register != nil {
		deps.Register(router, protected)
	}
	return router
}

func healthHandler(context *gin.Context) {
	context.JSON(http.StatusOK, gin.H{"status": "ok"})
}
