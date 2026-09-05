// Package config loads environment-driven settings for the server.
package config

import (
	"os"
	"strconv"
)

type Config struct {
	Bind        string
	DatabaseURL string
	FrontendURL string
	BaseURL     string

	JWTSecret     string
	SessionSecret string

	GoogleClientID     string
	GoogleClientSecret string

	MicrosoftClientID     string
	MicrosoftClientSecret string

	VisionDetectorPath      string
	VisionRecognizerPath    string
	VisionSharedLibraryPath string
	VisionUseGPU            bool
	VisionGPUDeviceID       int
}

func Load() Config {
	return Config{
		Bind:        getEnv("BIND", "0.0.0.0:8080"),
		DatabaseURL: getEnv("DATABASE_URL", "postgres://postgres:password@localhost:5432/trackid"),
		FrontendURL: getEnv("FRONTEND_URL", "http://localhost:5173"),
		BaseURL:     getEnv("BASE_URL", "http://localhost:8080"),

		JWTSecret:     getEnv("JWT_SECRET", "dev-secret-change-me"),
		SessionSecret: getEnv("SESSION_SECRET", "dev-session-secret-change-me"),

		GoogleClientID:     getEnv("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret: getEnv("GOOGLE_CLIENT_SECRET", ""),

		MicrosoftClientID:     getEnv("MICROSOFT_CLIENT_ID", ""),
		MicrosoftClientSecret: getEnv("MICROSOFT_CLIENT_SECRET", ""),

		VisionDetectorPath:      getEnv("VISION_DETECTOR_MODEL_PATH", "models/scrfd_10g_bnkps.onnx"),
		VisionRecognizerPath:    getEnv("VISION_RECOGNIZER_MODEL_PATH", "models/glintr100.onnx"),
		VisionSharedLibraryPath: getEnv("ONNXRUNTIME_SHARED_LIBRARY_PATH", ""),
		VisionUseGPU:            getEnvBool("VISION_USE_GPU", true),
		VisionGPUDeviceID:       getEnvInt("VISION_GPU_DEVICE_ID", 0),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
