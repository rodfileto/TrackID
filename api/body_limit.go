package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/rodfileto/trackid/video"
)

// DefaultMaxBodyBytes bounds every request body but those of largeBodyRoutes: well above the
// largest image, document or draft TrackID takes (handlers keep their own, smaller limits), and far
// below what would fill the server's temporary space.
const DefaultMaxBodyBytes = 128 << 20

// largeBodyRoutes take videos, up to video.MaxBytes each.
var largeBodyRoutes = map[string]bool{
	"/api/admin/cases/:caseId/requests/:requestId/material":       true,
	"/api/admin/cases/:caseId/collections/:collectionId/material": true,
	"/api/cases/:caseId/collections/:collectionId/material":       true,
	"/api/cases/:caseId/videos":                                   true,
	"/api/videolab/jobs":                                          true,
}

// maxLargeBodyBytes leaves room for the multipart envelope around a video of video.MaxBytes.
const maxLargeBodyBytes = video.MaxBytes + 1<<20

// limitBody refuses a body over its route's limit: at once, by Content-Length, when the client
// declares one, and otherwise when reading passes the limit.
func limitBody() gin.HandlerFunc {
	return func(c *gin.Context) {
		limit := int64(DefaultMaxBodyBytes)
		if largeBodyRoutes[c.FullPath()] {
			limit = maxLargeBodyBytes
		}
		if c.Request.ContentLength > limit {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"error": "the request is too large"})
			return
		}
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		}
		c.Next()
	}
}
