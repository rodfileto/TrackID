// Package web serves the built frontend (the prototype React app) as a
// single-page application. It exists so the API server can serve both the API
// and the prototype UI from one process, which makes the features easier to
// exercise. It reads the built bundle from disk (frontend/dist by default) and
// is intentionally tolerant of the build being absent — then it simply serves
// nothing at those routes, and the API keeps working.
package web

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// DistDir returns the filesystem path of the built frontend, or "" when no
// build is present. The path comes from TRACKID_WEB_DIST, falling back to
// frontend/dist under the current working directory (the repo root when run
// via scripts/start_dev.sh).
func DistDir() string {
	if dir := os.Getenv("TRACKID_WEB_DIST"); dir != "" {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
	}
	for _, candidate := range []string{"frontend/dist", "dist"} {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			if abs, err := filepath.Abs(candidate); err == nil {
				return abs
			}
			return candidate
		}
	}
	return ""
}

// SPAHandler serves distDir as a single-page application: files are served
// from disk, and any path that doesn't map to a file falls back to index.html
// so client-side routing works.
func SPAHandler(distDir string) http.Handler {
	fs := http.Dir(distDir)
	fileServer := http.FileServer(fs)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}
		path := strings.TrimPrefix(filepath.Clean("/"+r.URL.Path), "/")
		if path == "" || path == "." {
			path = "index.html"
		}
		if f, err := fs.Open(path); err != nil {
			if _, indexErr := fs.Open("index.html"); indexErr != nil {
				http.NotFound(w, r)
				return
			}
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
			return
		} else {
			_ = f.Close()
			fileServer.ServeHTTP(w, r)
		}
	})
}
