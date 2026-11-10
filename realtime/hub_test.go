package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func clientCount(h *Hub) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// connect opens a real WebSocket served by hub and waits until the hub has registered it.
func connect(t *testing.T, h *Hub, session Session) *websocket.Conn {
	t.Helper()
	before := clientCount(h)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		h.Serve(r.Context(), conn, session)
	}))
	t.Cleanup(server.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.CloseNow() })
	deadline := time.Now().Add(5 * time.Second)
	for clientCount(h) == before {
		if time.Now().After(deadline) {
			t.Fatal("the hub never registered the connection")
		}
		time.Sleep(time.Millisecond)
	}
	return conn
}

func readEvent(t *testing.T, conn *websocket.Conn, wait time.Duration) (Event, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	_, data, err := conn.Read(ctx)
	if err != nil {
		return Event{}, err
	}
	var event Event
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatal(err)
	}
	return event, nil
}

func wantClose(t *testing.T, conn *websocket.Conn, want websocket.StatusCode) {
	t.Helper()
	_, err := readEvent(t, conn, 5*time.Second)
	if got := websocket.CloseStatus(err); got != want {
		t.Fatalf("close status = %d (%v), want %d", got, err, want)
	}
}

func TestTicketWorksOnce(t *testing.T) {
	h := NewHub(Options{})
	expires := time.Now().Add(time.Hour)
	value := h.IssueTicket(7, expires)
	ticket, ok := h.RedeemTicket(value)
	if !ok || ticket.UserID != 7 || !ticket.TokenExpires.Equal(expires) {
		t.Fatalf("redeem = %+v, %t", ticket, ok)
	}
	if _, ok := h.RedeemTicket(value); ok {
		t.Fatal("a ticket was redeemed twice")
	}
	if _, ok := h.RedeemTicket("made-up"); ok {
		t.Fatal("an unknown ticket was accepted")
	}
}

func TestTicketExpires(t *testing.T) {
	h := NewHub(Options{TicketTTL: time.Millisecond})
	value := h.IssueTicket(7, time.Time{})
	time.Sleep(5 * time.Millisecond)
	if _, ok := h.RedeemTicket(value); ok {
		t.Fatal("an expired ticket was accepted")
	}
}

func TestPublishReachesOnlyTheTopic(t *testing.T) {
	h := NewHub(Options{})
	first := connect(t, h, Session{UserID: 1})
	secondTab := connect(t, h, Session{UserID: 1})
	other := connect(t, h, Session{UserID: 2})

	h.Publish(UserTopic(1), Event{Type: "hello"})
	for _, conn := range []*websocket.Conn{first, secondTab} {
		event, err := readEvent(t, conn, 5*time.Second)
		if err != nil || event.Type != "hello" || event.Topic != "user:1" {
			t.Fatalf("event = %+v, %v", event, err)
		}
	}
	if event, err := readEvent(t, other, 100*time.Millisecond); err == nil {
		t.Fatalf("another user received %+v", event)
	}
}

func TestResyncReachesEveryone(t *testing.T) {
	h := NewHub(Options{})
	one := connect(t, h, Session{UserID: 1})
	two := connect(t, h, Session{UserID: 2})
	h.Resync()
	for _, conn := range []*websocket.Conn{one, two} {
		if event, err := readEvent(t, conn, 5*time.Second); err != nil || event.Type != EventResync {
			t.Fatalf("event = %+v, %v", event, err)
		}
	}
}

// A client whose buffer is full is cut off rather than holding up everyone else.
func TestSlowClientIsKicked(t *testing.T) {
	h := NewHub(Options{BufferSize: 1})
	slow := &client{send: make(chan []byte, 1), topics: []string{UserTopic(1)}, kicked: make(chan struct{})}
	fast := &client{send: make(chan []byte, 8), topics: []string{UserTopic(1)}, kicked: make(chan struct{})}
	h.add(slow)
	h.add(fast)
	for range 3 {
		h.Publish(UserTopic(1), Event{Type: "x"})
	}
	select {
	case <-slow.kicked:
	default:
		t.Fatal("the slow client was not kicked")
	}
	if slow.status != websocket.StatusTryAgainLater {
		t.Fatalf("status = %d", slow.status)
	}
	if len(fast.send) != 3 {
		t.Fatalf("the other client got %d of 3 events", len(fast.send))
	}
}

func TestExpiredTokenCloses(t *testing.T) {
	h := NewHub(Options{})
	wantClose(t, connect(t, h, Session{UserID: 1, Expires: time.Now().Add(50 * time.Millisecond)}), CloseTokenExpired)
}

func TestRevokedAccessCloses(t *testing.T) {
	h := NewHub(Options{CheckInterval: 10 * time.Millisecond})
	conn := connect(t, h, Session{UserID: 1, Check: func(context.Context) error { return ErrAccessRevoked }})
	wantClose(t, conn, CloseAccessRevoked)
}

func TestFailedCheckAsksForARetry(t *testing.T) {
	h := NewHub(Options{CheckInterval: 10 * time.Millisecond})
	conn := connect(t, h, Session{UserID: 1, Check: func(context.Context) error { return errors.New("database down") }})
	wantClose(t, conn, websocket.StatusTryAgainLater)
}

func TestCloseDisconnectsAndRefuses(t *testing.T) {
	h := NewHub(Options{})
	conn := connect(t, h, Session{UserID: 1})
	h.Close()
	wantClose(t, conn, websocket.StatusGoingAway)
	if h.add(&client{topics: []string{UserTopic(1)}, kicked: make(chan struct{})}) {
		t.Fatal("a closed hub accepted a client")
	}
}

func TestDisconnectedClientIsForgotten(t *testing.T) {
	h := NewHub(Options{})
	conn := connect(t, h, Session{UserID: 1})
	conn.Close(websocket.StatusNormalClosure, "")
	deadline := time.Now().Add(5 * time.Second)
	for clientCount(h) != 0 {
		if time.Now().After(deadline) {
			t.Fatal("the client stayed registered")
		}
		time.Sleep(time.Millisecond)
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	if len(h.topics) != 0 {
		t.Fatalf("topics left behind: %v", h.topics)
	}
}
