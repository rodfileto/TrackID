package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/rodfileto/trackid/analyses"
	"github.com/rodfileto/trackid/auth"
	"github.com/rodfileto/trackid/cases"
	"github.com/rodfileto/trackid/distribution"
	"github.com/rodfileto/trackid/notifications"
	"github.com/rodfileto/trackid/realtime"
)

type notificationsResponse struct {
	Items  []notificationView `json:"items"`
	Unread int64              `json:"unread"`
}

// TestNotificationsOverWebSocket follows one assignment through the router: a ticket opens the
// socket, the assignment's commit reaches it, and the list shows only what the reader may see now.
func TestNotificationsOverWebSocket(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	sqlDB := testDB(t)
	repo := auth.NewRepository(sqlDB)
	user := func(name string, access auth.Access) auth.UserProfile {
		t.Helper()
		access.IsActive, access.AccountRole = true, auth.AccountRoleUser
		u, err := repo.Create(ctx, auth.NewUser{FirstName: name, LastName: "Test", StaffNumber: name, JobTitle: "test", Username: name,
			Password: "password1", Access: access}, "")
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	coordinator := user("coord", auth.Access{Coordinator: true})
	ana := user("ana", auth.Access{ExaminesFacial: true})
	bia := user("bia", auth.Access{ExaminesFacial: true})

	const secret = "test-secret"
	hub := realtime.NewHub(realtime.Options{})
	defer hub.Close()
	server := httptest.NewServer(NewRouter(Dependencies{DB: sqlDB, JWTSecret: secret, Hub: hub}))
	defer server.Close()
	publisher := notifications.Relay(sqlDB, hub)

	call := func(as auth.UserProfile, method, path string, out any) int {
		t.Helper()
		token, err := IssueToken(as, secret)
		if err != nil {
			t.Fatal(err)
		}
		request, err := http.NewRequestWithContext(ctx, method, server.URL+"/api"+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer "+token)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if out != nil {
			if err := json.NewDecoder(response.Body).Decode(out); err != nil {
				t.Fatal(err)
			}
		}
		return response.StatusCode
	}
	ticketFor := func(as auth.UserProfile) string {
		t.Helper()
		var body struct {
			Ticket string `json:"ticket"`
		}
		if status := call(as, http.MethodPost, "/realtime/ticket", &body); status != http.StatusOK || body.Ticket == "" {
			t.Fatalf("ticket: status %d, %+v", status, body)
		}
		return body.Ticket
	}
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/realtime?ticket="
	list := func(as auth.UserProfile) notificationsResponse {
		t.Helper()
		var body notificationsResponse
		if status := call(as, http.MethodGet, "/notifications", &body); status != http.StatusOK {
			t.Fatalf("list: status %d", status)
		}
		return body
	}
	assign := func(analysisID int64, to auth.UserProfile) {
		t.Helper()
		if err := distribution.Assign(ctx, sqlDB, analysisID, &to.ID, coordinator.ID); err != nil {
			t.Fatal(err)
		}
	}

	// A ticket is the only way in, and only from this origin.
	if status := dialStatus(ctx, wsURL+"made-up", nil); status != http.StatusUnauthorized {
		t.Fatalf("made-up ticket: status %d", status)
	}
	foreign := http.Header{"Origin": {"https://elsewhere.example"}}
	if status := dialStatus(ctx, wsURL+ticketFor(ana), foreign); status != http.StatusForbidden {
		t.Fatalf("foreign origin: status %d", status)
	}

	ticket := ticketFor(ana)
	conn, _, err := websocket.Dial(ctx, wsURL+ticket, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	if status := dialStatus(ctx, wsURL+ticket, nil); status != http.StatusUnauthorized {
		t.Fatalf("reused ticket: status %d", status)
	}

	if _, err := cases.Ingest(ctx, sqlDB, cases.CaseInput{CaseID: "N1", CaseType: cases.CaseTypeCriminal, Modality: "FACIAL", Description: "secret description"}); err != nil {
		t.Fatal(err)
	}
	analysisID := openTask(t, sqlDB, "N1", analyses.FacialReview)
	assign(analysisID, ana)

	items, _, err := notifications.List(ctx, sqlDB, ana.ID)
	if err != nil || len(items) != 1 {
		t.Fatalf("stored = %+v, %v", items, err)
	}
	// The relay is what the server's listener calls; the listener has its own test.
	publisher(ctx, fmt.Sprint(items[0].ID))
	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), notifications.EventChanged) || strings.Contains(string(data), "N1") {
		t.Fatalf("event = %s", data)
	}

	page := list(ana)
	if page.Unread != 1 || len(page.Items) != 1 {
		t.Fatalf("page = %+v", page)
	}
	item := page.Items[0]
	if item.CaseID != "N1" || item.AnalysisID != analysisID || item.AnalysisType != analyses.FacialReview || item.Step != analyses.StepVerificator {
		t.Fatalf("item = %+v", item)
	}
	if other := list(bia); len(other.Items) != 0 || other.Unread != 0 {
		t.Fatalf("bia sees %+v", other)
	}

	// Another user can't mark it, and the reader can.
	if status := call(bia, http.MethodPut, fmt.Sprintf("/notifications/%d/read", item.ID), nil); status != http.StatusNoContent {
		t.Fatalf("bia marking: status %d", status)
	}
	if page := list(ana); page.Unread != 1 {
		t.Fatalf("bia changed ana's notification: %+v", page)
	}
	if status := call(ana, http.MethodPut, fmt.Sprintf("/notifications/%d/read", item.ID), nil); status != http.StatusNoContent {
		t.Fatalf("marking: status %d", status)
	}
	if page := list(ana); page.Unread != 0 || !page.Items[0].Read {
		t.Fatalf("after marking: %+v", page)
	}

	// Once the work moves on, the notification keeps its place but says nothing about the case.
	assign(analysisID, bia)
	moved := list(ana).Items[0]
	if moved.CaseID != "" || moved.AnalysisID != 0 || moved.AnalysisType != "" {
		t.Fatalf("moved item = %+v", moved)
	}

	// Blind work is flagged, so the bell shows no case identity.
	if _, err := sqlDB.ExecContext(ctx, `UPDATE case_analyses SET stage = 'VERIFICATION' WHERE id = $1`, analysisID); err != nil {
		t.Fatal(err)
	}
	assign(analysisID, ana)
	blind := list(ana).Items[0]
	if !blind.Blind || blind.Step != analyses.StepReviewer {
		t.Fatalf("blind item = %+v", blind)
	}

	// A deactivated account can't get a ticket.
	if _, err := sqlDB.ExecContext(ctx, `UPDATE users SET is_active = FALSE WHERE id = $1`, ana.ID); err != nil {
		t.Fatal(err)
	}
	if status := call(ana, http.MethodPost, "/realtime/ticket", nil); status != http.StatusForbidden {
		t.Fatalf("deactivated ticket: status %d", status)
	}
}

func dialStatus(ctx context.Context, url string, header http.Header) int {
	conn, response, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: header})
	if err == nil {
		conn.CloseNow()
		return http.StatusSwitchingProtocols
	}
	if response == nil {
		return 0
	}
	return response.StatusCode
}
