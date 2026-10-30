package api

import (
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/rodfileto/trackid/imageproc"
)

// The labs (image lab, face lab) keep uploaded originals in memory for a while, so each preview
// need not upload them again. Nothing is stored.

const (
	imageLabMaxBytes   = 40 << 20
	imageLabMaxSources = 6
	imageLabSourceTTL  = time.Hour
)

type labSource struct {
	original imageproc.Original
	filename string
	used     time.Time
	// The face lab's: the file's EXIF fields and the faces detected in it.
	exif  imageproc.Exif
	faces []labFace
}

type labStore struct {
	mu      sync.Mutex
	sources map[string]*labSource
}

func newLabStore() *labStore { return &labStore{sources: map[string]*labSource{}} }

func (lab *labStore) put(s *labSource) {
	lab.mu.Lock()
	defer lab.mu.Unlock()
	now := time.Now()
	for id, s := range lab.sources {
		if now.Sub(s.used) > imageLabSourceTTL {
			delete(lab.sources, id)
		}
	}
	for len(lab.sources) >= imageLabMaxSources {
		var oldest string
		for id, s := range lab.sources {
			if oldest == "" || s.used.Before(lab.sources[oldest].used) {
				oldest = id
			}
		}
		delete(lab.sources, oldest)
	}
	s.used = now
	lab.sources[s.original.SHA256] = s
}

func (lab *labStore) get(id string) (*labSource, bool) {
	lab.mu.Lock()
	defer lab.mu.Unlock()
	s, ok := lab.sources[id]
	if ok {
		s.used = time.Now()
	}
	return s, ok
}

// source finds the request's original, answering 410 when it has expired so the page uploads
// it again.
func (lab *labStore) source(c *gin.Context, id string) (*labSource, bool) {
	s, ok := lab.get(id)
	if !ok {
		c.JSON(http.StatusGone, gin.H{"error": "the image is no longer in memory; send it again", "code": "source_expired"})
	}
	return s, ok
}

// readUpload reads the multipart "file", answering the error itself when it cannot.
func readUpload(c *gin.Context) (data []byte, filename string, ok bool) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return nil, "", false
	}
	if file.Size > imageLabMaxBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "the image is larger than 40 MB"})
		return nil, "", false
	}
	opened, err := file.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not read the file"})
		return nil, "", false
	}
	defer opened.Close()
	data, err = io.ReadAll(opened)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not read the file"})
		return nil, "", false
	}
	return data, file.Filename, true
}
