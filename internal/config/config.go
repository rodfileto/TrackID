package config

import (
	"os"
)

type Config struct {
	Port        string
	DatabaseURL string
	RedisURL    string
	JWTSecret   string
	Neo4jURL    string
	EmailDomain string
	S3Endpoint  string
	S3AccessKey string
	S3SecretKey string
	S3Bucket    string

	// Vision (cmd/worker only -- see embedding.ComputeForCodification). Left
	// empty, the worker logs a warning and skips registering the embedding
	// task handler rather than failing to start.
	VisionDetectorPath      string
	VisionRecognizerPath    string
	VisionSharedLibraryPath string
	VisionUseGPU            bool
}

func Load() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
	}

	return Config{
		Port:        port,
		DatabaseURL: os.Getenv("DATABASE_URL"),
		RedisURL:    os.Getenv("REDIS_URL"),
		JWTSecret:   os.Getenv("JWT_SECRET"),
		Neo4jURL:    os.Getenv("NEO4J_URL"),
		EmailDomain: os.Getenv("EMAIL_DOMAIN"),
		S3Endpoint:  os.Getenv("S3_ENDPOINT"),
		S3AccessKey: os.Getenv("S3_ACCESS_KEY"),
		S3SecretKey: os.Getenv("S3_SECRET_KEY"),
		S3Bucket:    os.Getenv("S3_BUCKET"),

		VisionDetectorPath:      os.Getenv("VISION_DETECTOR_PATH"),
		VisionRecognizerPath:    os.Getenv("VISION_RECOGNIZER_PATH"),
		VisionSharedLibraryPath: os.Getenv("VISION_SHARED_LIBRARY_PATH"),
		VisionUseGPU:            os.Getenv("VISION_USE_GPU") == "true",
	}
}
