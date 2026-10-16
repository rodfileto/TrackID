package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/rodfileto/trackid/auth"
)

func TestPublicRegistrationIsGone(t *testing.T) {
	recorder := httptest.NewRecorder()
	NewRouter(Dependencies{JWTSecret: testJWTSecret}).ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/auth/register", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, recorder.Code)
	}
}

func TestAdminUsersWithoutDatabase(t *testing.T) {
	recorder := httptest.NewRecorder()
	NewRouter(Dependencies{JWTSecret: testJWTSecret}).ServeHTTP(recorder, authorizedRequest(t, http.MethodGet, "/api/admin/users"))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, recorder.Code)
	}
}

func TestRequireAdmin(t *testing.T) {
	for _, tt := range []struct {
		role auth.AccountRole
		want int
	}{
		{auth.AccountRoleUser, http.StatusForbidden},
		{auth.AccountRoleAdmin, http.StatusOK},
		{auth.AccountRoleSuperAdmin, http.StatusOK},
	} {
		router := gin.New()
		router.GET("/", func(context *gin.Context) {
			context.Set(currentUserKey, auth.UserProfile{AccountRole: tt.role, IsActive: true})
		}, RequireAdmin(), func(context *gin.Context) { context.Status(http.StatusOK) })
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
		if recorder.Code != tt.want {
			t.Errorf("%s: expected status %d, got %d", tt.role, tt.want, recorder.Code)
		}
	}
}
