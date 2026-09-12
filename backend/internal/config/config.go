package config

import (
	"os"
	"strings"

	"github.com/rodrigorfcm/trackid/backend/internal/infobio"
)

type Config struct {
	Port               string
	DatabaseURL        string
	RedisURL           string
	JWTSecret          string
	Neo4jURL           string
	InfoBioBaseURL     string
	InfoBioImagesURL   string
	InfoBioNISTBaseURL string
}

func Load() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	infoBioBaseURL := os.Getenv("INFOBIO_BASE_URL")
	if strings.TrimSpace(infoBioBaseURL) == "" {
		infoBioBaseURL = infobio.DefaultBaseURL
	}
	infoBioImagesURL := os.Getenv("INFOBIO_IMAGES_URL")
	if strings.TrimSpace(infoBioImagesURL) == "" {
		infoBioImagesURL = infobio.DefaultImagesURL
	}
	infoBioNISTBaseURL := os.Getenv("INFOBIO_NIST_BASE_URL")
	if strings.TrimSpace(infoBioNISTBaseURL) == "" {
		infoBioNISTBaseURL = infobio.DefaultNISTBaseURL
	}

	return Config{
		Port:               port,
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		RedisURL:           os.Getenv("REDIS_URL"),
		JWTSecret:          os.Getenv("JWT_SECRET"),
		Neo4jURL:           os.Getenv("NEO4J_URL"),
		InfoBioBaseURL:     infoBioBaseURL,
		InfoBioImagesURL:   infoBioImagesURL,
		InfoBioNISTBaseURL: infoBioNISTBaseURL,
	}
}
