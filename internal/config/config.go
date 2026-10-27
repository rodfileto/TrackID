package config

import (
	"log"
	"os"
	"time"
)

type Config struct {
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

	return Config{
		Port:        port,
		DatabaseURL: os.Getenv("DATABASE_URL"),
		RedisURL:    os.Getenv("REDIS_URL"),
		JWTSecret:   os.Getenv("JWT_SECRET"),
		EmailDomain: os.Getenv("EMAIL_DOMAIN"),
		S3Endpoint:  os.Getenv("S3_ENDPOINT"),
		S3AccessKey: os.Getenv("S3_ACCESS_KEY"),
		S3SecretKey: os.Getenv("S3_SECRET_KEY"),
		S3Bucket:    os.Getenv("S3_BUCKET"),

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
	}
}
