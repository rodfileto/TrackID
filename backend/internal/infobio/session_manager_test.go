package infobio

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateUserSessionCapturesRedirectCookies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case loginPath:
			http.SetCookie(responseWriter, &http.Cookie{Name: "PHPSESSID", Value: "session-123", Path: "/"})
			responseWriter.Header().Set("Location", "/infobio/home")
			responseWriter.WriteHeader(http.StatusFound)
		case "/infobio/home":
			fmt.Fprint(responseWriter, "logged in")
		default:
			http.NotFound(responseWriter, request)
		}
	}))
	defer server.Close()

	store := NewMemorySessionStore()
	manager := NewSessionManager(server.URL, server.Client(), store)
	if err := manager.CreateUserSession(context.Background(), 42, "user", "password"); err != nil {
		t.Fatal(err)
	}

	session, ok, err := store.Load(context.Background(), 42)
	if err != nil || !ok {
		t.Fatalf("stored session missing: ok=%v err=%v", ok, err)
	}
	if got := session.Cookies["PHPSESSID"]; got != "session-123" {
		t.Fatalf("PHPSESSID = %q, want %q", got, "session-123")
	}
}
