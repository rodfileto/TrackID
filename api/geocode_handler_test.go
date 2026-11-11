package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/rodfileto/trackid/geocode"
)

type nearGeocoder struct{ asked *geocode.Query }

func (g *nearGeocoder) Search(_ context.Context, q geocode.Query) ([]geocode.Result, error) {
	g.asked = &q
	return nil, geocode.ErrNotFound
}

func (g *nearGeocoder) Reverse(context.Context, float64, float64) (geocode.Result, error) {
	return geocode.Result{}, geocode.ErrNotFound
}

func TestGeocodeSearchPassesTheMapsCentre(t *testing.T) {
	gin.SetMode(gin.TestMode)
	g := &nearGeocoder{}
	router := gin.New()
	router.GET("/search", GeocodeSearchHandler(g))

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/search?q=Rua+Goi%C3%A1s+120&near=-43.94,-19.92", nil))
	if recorder.Code != http.StatusOK || g.asked == nil || g.asked.Near == nil || *g.asked.Near != (geocode.Point{Lon: -43.94, Lat: -19.92}) {
		t.Fatalf("near not passed: %d %+v", recorder.Code, g.asked)
	}

	g.asked = nil
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/search?q=Rua+A", nil))
	if recorder.Code != http.StatusOK || g.asked == nil || g.asked.Near != nil {
		t.Fatalf("no near: %d %+v", recorder.Code, g.asked)
	}

	for _, near := range []string{"x", "-43.9", "200,10", "-43.9,abc"} {
		recorder = httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/search?q=Rua+A&near="+near, nil))
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("near=%s: %d", near, recorder.Code)
		}
	}
}
