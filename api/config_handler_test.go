package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestConfigHandlerServesClientConfigWithoutSignIn(t *testing.T) {
	deps := Dependencies{JWTSecret: testJWTSecret, ClientConfig: ClientConfig{
		MapTileURL: "https://tiles.example/{z}/{x}/{y}.png", MapTileAttribution: "© Example",
	}}
	recorder := httptest.NewRecorder()
	NewRouter(deps).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/config", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	var got ClientConfig
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got != deps.ClientConfig {
		t.Fatalf("got %+v, want %+v", got, deps.ClientConfig)
	}
}

func TestConfigHandlerLeavesUnsetFieldsOut(t *testing.T) {
	recorder := httptest.NewRecorder()
	NewRouter(Dependencies{JWTSecret: testJWTSecret}).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/config", nil))

	if body := recorder.Body.String(); body != "{}" {
		t.Fatalf("expected an empty object, got %s", body)
	}
}
