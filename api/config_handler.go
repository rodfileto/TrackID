package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ClientConfig is what the frontend reads at startup (GET /api/config), so one build of it fits
// every deployment: settings that would otherwise be fixed when the bundle is built. Empty fields
// are left out, and the frontend uses its own default for them. Nothing here is secret, since it
// is served before sign-in.
type ClientConfig struct {
	// MapTileURL is the base map's raster tile URL template ("https://tiles.example/{z}/{x}/{y}.png").
	// Empty means OpenStreetMap's public tile servers, which allow light use only.
	MapTileURL string `json:"mapTileUrl,omitempty"`
	// MapTileAttribution is the credit shown on the map, as HTML.
	MapTileAttribution string `json:"mapTileAttribution,omitempty"`
}

// ConfigHandler serves the frontend's runtime configuration.
func ConfigHandler(config ClientConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-cache")
		c.JSON(http.StatusOK, config)
	}
}
