package main

import (
	"context"
	"log"

	"github.com/gin-gonic/gin"

	"trackid-backend/internal/auth"
	"trackid-backend/internal/config"
	"trackid-backend/internal/db"
	"trackid-backend/internal/handlers"
	"trackid-backend/internal/jobs"
	"trackid-backend/internal/oauth"
	"github.com/rodfileto/trackid-vision/videoproc"
	"github.com/rodfileto/trackid-vision/vision"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()

	database, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect to database: %v", err)
	}
	if err := database.EnsureSchema(ctx); err != nil {
		log.Fatalf("ensure schema: %v", err)
	}

	authSvc := auth.NewService(cfg.JWTSecret)
	oauth.Setup(cfg)
	oauthHandler := oauth.NewHandler(database, authSvc, cfg.FrontendURL)

	visionSvc, err := vision.NewService(vision.Config{
		DetectorPath:      cfg.VisionDetectorPath,
		RecognizerPath:    cfg.VisionRecognizerPath,
		SharedLibraryPath: cfg.VisionSharedLibraryPath,
		UseGPU:            cfg.VisionUseGPU,
		DeviceID:          cfg.VisionGPUDeviceID,
	})
	if err != nil {
		log.Fatalf("initialize vision service: %v", err)
	}
	defer visionSvc.Close()

	jobStore := jobs.NewStore[videoproc.Result]()

	router := gin.Default()
	handlers.RegisterRoutes(router, handlers.Deps{
		DB:      database,
		AuthSvc: authSvc,
		OAuth:   oauthHandler,
		Jobs:    jobStore,
		Vision:  visionSvc,
	})

	log.Printf("listening on %s", cfg.Bind)
	if err := router.Run(cfg.Bind); err != nil {
		log.Fatal(err)
	}
}
