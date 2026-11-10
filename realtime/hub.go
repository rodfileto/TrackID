// Package realtime pushes events to connected browsers over WebSockets (docs/realtime-plan.md).
// The socket is only a signal: anything that must not be lost is committed to Postgres first, and a
// client that missed a push recovers by fetching from there.
//
// Connections live in the memory of the process that accepted them, so delivery reaches only the
// clients of this API instance. Handlers depend on Publisher, so a message bus can take over if a
// second instance ever runs.
package realtime

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// Close codes the frontend acts on (services/realtime.ts).
const (
	// CloseTokenExpired ends a connection when its JWT expires: reconnect once there is a new token.
	CloseTokenExpired websocket.StatusCode = 4001
	// CloseAccessRevoked ends a connection whose account lost access: do not reconnect.
	CloseAccessRevoked websocket.StatusCode = 4003
)

// EventResync tells clients that pushes may have been missed, so they should fetch what they show.
const EventResync = "resync"

// ErrAccessRevoked is what Session.Check returns when the connection's user may no longer stay.
var ErrAccessRevoked = errors.New("access revoked")

// Event is one message to the browser.
type Event struct {
	Topic string `json:"topic,omitempty"`
	Type  string `json:"type"`
	Data  any    `json:"data,omitempty"`
}

// Publisher delivers events to whoever follows a topic.
type Publisher interface {
	Publish(topic string, event Event)
}

// UserTopic is the topic every connection of one user follows.
func UserTopic(userID int64) string {
	return "user:" + strconv.FormatInt(userID, 10)
}

// Options tunes a Hub. Zero values take the defaults.
type Options struct {
	TicketTTL     time.Duration // how long a ticket can wait to be redeemed (30s)
	PingInterval  time.Duration // how often an idle connection is probed (30s)
	CheckInterval time.Duration // how often Session.Check runs (60s)
	WriteTimeout  time.Duration // how long one write, ping or check can take (10s)
	BufferSize    int           // events waiting per client before it counts as too slow (64)
}

// Ticket is what a redeemed ticket proves: who asked for it, and when their token ends.
type Ticket struct {
	UserID       int64
	TokenExpires time.Time
	validUntil   time.Time
}

// Session is one authenticated connection for Serve.
type Session struct {
	UserID  int64
	Expires time.Time // when the JWT ends; zero if it has no expiry
	// Check re-validates access, since middleware runs only once on a long connection.
	// ErrAccessRevoked closes the connection for good; any other error makes the client retry.
	Check func(context.Context) error
}

// Hub maps topics to connected clients. Publish never blocks: a client that can't keep up is
// disconnected and resyncs when it reconnects.
type Hub struct {
	options Options

	mu      sync.RWMutex
	topics  map[string]map[*client]struct{}
	clients map[*client]struct{}
	closed  bool

	ticketsMu sync.Mutex
	tickets   map[string]Ticket
}

type client struct {
	send     chan []byte
	topics   []string
	kicked   chan struct{}
	kickOnce sync.Once
	status   websocket.StatusCode // set before kicked closes
	reason   string
}

func (c *client) kick(status websocket.StatusCode, reason string) {
	c.kickOnce.Do(func() {
		c.status, c.reason = status, reason
		close(c.kicked)
	})
}

// NewHub returns an empty hub.
func NewHub(options Options) *Hub {
	if options.TicketTTL == 0 {
		options.TicketTTL = 30 * time.Second
	}
	if options.PingInterval == 0 {
		options.PingInterval = 30 * time.Second
	}
	if options.CheckInterval == 0 {
		options.CheckInterval = time.Minute
	}
	if options.WriteTimeout == 0 {
		options.WriteTimeout = 10 * time.Second
	}
	if options.BufferSize == 0 {
		options.BufferSize = 64
	}
	return &Hub{
		options: options,
		topics:  map[string]map[*client]struct{}{},
		clients: map[*client]struct{}{},
		tickets: map[string]Ticket{},
	}
}

// IssueTicket returns a one-time ticket to open a WebSocket. Browsers can't set Authorization on a
// WebSocket, and a token in a URL ends up in logs, so the token buys this short-lived stand-in.
func (h *Hub) IssueTicket(userID int64, tokenExpires time.Time) string {
	now := time.Now()
	value := rand.Text()
	h.ticketsMu.Lock()
	defer h.ticketsMu.Unlock()
	for key, ticket := range h.tickets {
		if now.After(ticket.validUntil) {
			delete(h.tickets, key)
		}
	}
	h.tickets[value] = Ticket{UserID: userID, TokenExpires: tokenExpires, validUntil: now.Add(h.options.TicketTTL)}
	return value
}

// RedeemTicket consumes a ticket. It works once, and only before the ticket expires.
func (h *Hub) RedeemTicket(value string) (Ticket, bool) {
	h.ticketsMu.Lock()
	defer h.ticketsMu.Unlock()
	ticket, ok := h.tickets[value]
	if !ok {
		return Ticket{}, false
	}
	delete(h.tickets, value)
	if time.Now().After(ticket.validUntil) {
		return Ticket{}, false
	}
	return ticket, true
}

// Publish sends event to every client following topic.
func (h *Hub) Publish(topic string, event Event) {
	event.Topic = topic
	message, ok := encode(event)
	if !ok {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.topics[topic] {
		deliver(c, message)
	}
}

// Resync tells every client to fetch what it shows again.
func (h *Hub) Resync() {
	message, ok := encode(Event{Type: EventResync})
	if !ok {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		deliver(c, message)
	}
}

func encode(event Event) ([]byte, bool) {
	message, err := json.Marshal(event)
	if err != nil {
		log.Printf("realtime: could not encode a %s event: %v", event.Type, err)
		return nil, false
	}
	return message, true
}

func deliver(c *client, message []byte) {
	select {
	case c.send <- message:
	default:
		c.kick(websocket.StatusTryAgainLater, "too slow")
	}
}

// Close disconnects every client and refuses new ones.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	for c := range h.clients {
		c.kick(websocket.StatusGoingAway, "server shutting down")
	}
}

func (h *Hub) add(c *client) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return false
	}
	h.clients[c] = struct{}{}
	for _, topic := range c.topics {
		if h.topics[topic] == nil {
			h.topics[topic] = map[*client]struct{}{}
		}
		h.topics[topic][c] = struct{}{}
	}
	return true
}

func (h *Hub) remove(c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, c)
	for _, topic := range c.topics {
		delete(h.topics[topic], c)
		if len(h.topics[topic]) == 0 {
			delete(h.topics, topic)
		}
	}
}

// Serve runs one connection until it ends. For now the browser only listens, so any message it
// sends closes the connection (websocket.Conn.CloseRead).
func (h *Hub) Serve(ctx context.Context, conn *websocket.Conn, session Session) {
	defer conn.CloseNow()
	ctx = conn.CloseRead(ctx)

	c := &client{
		send:   make(chan []byte, h.options.BufferSize),
		topics: []string{UserTopic(session.UserID)},
		kicked: make(chan struct{}),
	}
	if !h.add(c) {
		conn.Close(websocket.StatusGoingAway, "server shutting down")
		return
	}
	defer h.remove(c)

	ping := time.NewTicker(h.options.PingInterval)
	defer ping.Stop()
	check := time.NewTicker(h.options.CheckInterval)
	defer check.Stop()
	var expired <-chan time.Time
	if !session.Expires.IsZero() {
		timer := time.NewTimer(time.Until(session.Expires))
		defer timer.Stop()
		expired = timer.C
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.kicked:
			conn.Close(c.status, c.reason)
			return
		case <-expired:
			conn.Close(CloseTokenExpired, "token expired")
			return
		case message := <-c.send:
			if err := h.bounded(ctx, func(ctx context.Context) error {
				return conn.Write(ctx, websocket.MessageText, message)
			}); err != nil {
				return
			}
		case <-ping.C:
			if err := h.bounded(ctx, conn.Ping); err != nil {
				return
			}
		case <-check.C:
			if session.Check == nil {
				continue
			}
			err := h.bounded(ctx, session.Check)
			if errors.Is(err, ErrAccessRevoked) {
				conn.Close(CloseAccessRevoked, "access revoked")
				return
			}
			if err != nil {
				conn.Close(websocket.StatusTryAgainLater, "could not verify access")
				return
			}
		}
	}
}

func (h *Hub) bounded(ctx context.Context, operation func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, h.options.WriteTimeout)
	defer cancel()
	return operation(ctx)
}
