package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func requestWithLength(method, path string, length int64) *http.Request {
	request := httptest.NewRequest(method, path, strings.NewReader("{}"))
	request.ContentLength = length
	return request
}

func TestLimitBodyRefusesALargeBodyOnAnOrdinaryRoute(t *testing.T) {
	recorder := httptest.NewRecorder()
	NewRouter(Dependencies{JWTSecret: testJWTSecret}).ServeHTTP(recorder,
		requestWithLength(http.MethodPost, "/api/cases/C-1/evidences", DefaultMaxBodyBytes+1))
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", recorder.Code)
	}
}

func TestLimitBodyLetsAVideoRouteTakeMore(t *testing.T) {
	recorder := httptest.NewRecorder()
	NewRouter(Dependencies{JWTSecret: testJWTSecret}).ServeHTTP(recorder,
		requestWithLength(http.MethodPost, "/api/cases/C-1/videos", DefaultMaxBodyBytes+1))
	// Past the limit, the route's own checks answer: here, sign-in.
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 from sign-in, got %d", recorder.Code)
	}
	recorder = httptest.NewRecorder()
	NewRouter(Dependencies{JWTSecret: testJWTSecret}).ServeHTTP(recorder,
		requestWithLength(http.MethodPost, "/api/cases/C-1/videos", maxLargeBodyBytes+1))
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 past a video's limit, got %d", recorder.Code)
	}
}

// Every route whose handler takes videos must be in largeBodyRoutes, or videos sent to it would be
// refused at DefaultMaxBodyBytes; every entry must still name a route.
func TestLargeBodyRoutesMatchTheVideoHandlers(t *testing.T) {
	takesVideos := []string{"api.receiveMaterial.", "api.AddVideoHandler.", "api.(*VideoLab).UploadHandler."}
	registered := map[string]bool{}
	for _, route := range NewRouter(Dependencies{JWTSecret: testJWTSecret}).Routes() {
		registered[route.Path] = true
		if route.Method != http.MethodPost {
			continue
		}
		for _, name := range takesVideos {
			if strings.Contains(route.Handler, name) && !largeBodyRoutes[route.Path] {
				t.Errorf("%s %s (%s) takes videos but isn't in largeBodyRoutes", route.Method, route.Path, route.Handler)
			}
		}
	}
	for path := range largeBodyRoutes {
		if !registered[path] {
			t.Errorf("largeBodyRoutes names %s, which no route has", path)
		}
	}
}

func TestReadyWithoutADatabase(t *testing.T) {
	recorder := httptest.NewRecorder()
	NewRouter(Dependencies{JWTSecret: testJWTSecret}).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", recorder.Code)
	}
	if body := recorder.Body.String(); !strings.Contains(body, `"database":"not configured"`) || !strings.Contains(body, `"storage":"not configured"`) {
		t.Fatalf("unexpected body %s", body)
	}
}
