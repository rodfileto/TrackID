// Package teams tells users of their notifications in Microsoft Teams, through a webhook each user sets: a Power
// Automate flow ("post card in a chat or channel when a webhook request is received") that takes an Adaptive Card at
// its root (docs/follow-plan.md, "Teams").
//
// Teams is outside TrackID, so a message says only what kind of notification is waiting, with a button that opens
// the app: no person, cluster or case, not even in the link.
package teams

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/notifications"
)

const (
	sendTimeout = 10 * time.Second
	// regroupInterval is how long a grouped notification that keeps changing waits before Teams is told again.
	regroupInterval = 15 * time.Minute
	queueSize       = 256
	senders         = 2
)

// DefaultHosts are the host suffixes a webhook may point at when TEAMS_WEBHOOK_HOSTS is empty: Power Automate's, and
// the older Office 365 connectors'.
var DefaultHosts = []string{".logic.azure.com", ".powerplatform.com", ".webhook.office.com"}

// Errors returned to callers.
var (
	ErrOff        = errors.New("Teams is not configured on this server")
	ErrInvalidURL = errors.New("the webhook must be an https URL of Power Automate")
	ErrNoWebhook  = errors.New("no Teams webhook is set")
)

// Settings configure Teams.
type Settings struct {
	// SecretsKey encrypts webhooks: 32 bytes, in base64 or hex. Empty turns Teams off.
	SecretsKey string
	// AppURL is the frontend's base URL, which a message's button opens.
	AppURL string
	// Hosts are the host suffixes a webhook may point at; empty means DefaultHosts.
	Hosts []string
	// Language is the messages' language: "en", or Brazilian Portuguese otherwise.
	Language string
}

// Service sets users' webhooks and sends their messages. A nil *Service is off.
type Service struct {
	db       *sql.DB
	aead     cipher.AEAD
	appURL   string
	hosts    []string
	language string
	client   *http.Client
	queue    chan int64
}

// New returns the service, or nil with a reason when Teams can't run.
func New(sqlDB *sql.DB, s Settings) (*Service, error) {
	if sqlDB == nil || s.SecretsKey == "" {
		return nil, errors.New("SECRETS_KEY is not set")
	}
	key, err := decodeKey(s.SecretsKey)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	hosts := s.Hosts
	if len(hosts) == 0 {
		hosts = DefaultHosts
	}
	return &Service{
		db: sqlDB, aead: aead, appURL: strings.TrimRight(s.AppURL, "/"), hosts: hosts, language: s.Language,
		client: &http.Client{
			Timeout: sendTimeout,
			// A redirect could lead anywhere: the allowlist is checked on the URL the user set only.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		queue: make(chan int64, queueSize),
	}, nil
}

// ParseHosts splits TEAMS_WEBHOOK_HOSTS.
func ParseHosts(value string) []string {
	var hosts []string
	for _, h := range strings.Split(value, ",") {
		if h = strings.ToLower(strings.TrimSpace(h)); h != "" {
			hosts = append(hosts, h)
		}
	}
	return hosts
}

func decodeKey(value string) ([]byte, error) {
	for _, decode := range []func(string) ([]byte, error){base64.StdEncoding.DecodeString, hex.DecodeString} {
		if key, err := decode(strings.TrimSpace(value)); err == nil && len(key) == 32 {
			return key, nil
		}
	}
	return nil, errors.New("SECRETS_KEY must be 32 bytes, in base64 or hex")
}

// checkURL refuses anything but https to an allowed host, since the server posts to what a user typed.
func (s *Service) checkURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.User != nil || u.Host == "" {
		return ErrInvalidURL
	}
	host := strings.ToLower(u.Hostname())
	for _, suffix := range s.hosts {
		if host == strings.TrimPrefix(suffix, ".") || strings.HasSuffix(host, "."+strings.TrimPrefix(suffix, ".")) {
			return nil
		}
	}
	return ErrInvalidURL
}

func (s *Service) seal(plain string) ([]byte, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return s.aead.Seal(nonce, nonce, []byte(plain), nil), nil
}

func (s *Service) open(sealed []byte) (string, error) {
	n := s.aead.NonceSize()
	if len(sealed) < n {
		return "", errors.New("the stored webhook is unreadable")
	}
	plain, err := s.aead.Open(nil, sealed[:n], sealed[n:], nil)
	if err != nil {
		return "", errors.New("the stored webhook is unreadable: was SECRETS_KEY changed?")
	}
	return string(plain), nil
}

// Status is what a user is shown about their webhook. The URL itself is never shown again.
type Status struct {
	Available  bool       `json:"available"`
	Configured bool       `json:"configured"`
	SetAt      *time.Time `json:"setAt,omitempty"`
	LastError  string     `json:"lastError,omitempty"`
	FailedAt   *time.Time `json:"failedAt,omitempty"`
}

// GetStatus returns the user's webhook status.
func (s *Service) GetStatus(ctx context.Context, userID int64) (Status, error) {
	if s == nil {
		return Status{}, nil
	}
	row, err := db.New(s.db).GetTeamsWebhook(ctx, userID)
	if err != nil {
		return Status{}, err
	}
	st := Status{Available: true, Configured: len(row.TeamsWebhook) > 0, LastError: row.TeamsLastError.String}
	if row.TeamsWebhookSetAt.Valid {
		st.SetAt = &row.TeamsWebhookSetAt.Time
	}
	if row.TeamsFailedAt.Valid {
		st.FailedAt = &row.TeamsFailedAt.Time
	}
	return st, nil
}

// SetWebhook stores the user's webhook, encrypted.
func (s *Service) SetWebhook(ctx context.Context, userID int64, raw string) error {
	if s == nil {
		return ErrOff
	}
	if err := s.checkURL(raw); err != nil {
		return err
	}
	sealed, err := s.seal(strings.TrimSpace(raw))
	if err != nil {
		return err
	}
	return db.New(s.db).SetTeamsWebhook(ctx, db.SetTeamsWebhookParams{ID: userID, TeamsWebhook: sealed})
}

// ClearWebhook removes the user's webhook.
func (s *Service) ClearWebhook(ctx context.Context, userID int64) error {
	if s == nil {
		return ErrOff
	}
	return db.New(s.db).ClearTeamsWebhook(ctx, userID)
}

// SendTest sends the user a test message now and records how it went.
func (s *Service) SendTest(ctx context.Context, userID int64) error {
	if s == nil {
		return ErrOff
	}
	row, err := db.New(s.db).GetTeamsWebhook(ctx, userID)
	if err != nil {
		return err
	}
	if len(row.TeamsWebhook) == 0 {
		return ErrNoWebhook
	}
	return s.deliver(ctx, userID, row.TeamsWebhook, s.text("test"))
}

// Notify queues a Teams message for a notification that is new or has changed, by the ID notifications.Relay
// receives. It never blocks: when the queue is full the message is dropped, and the bell still has the notification.
func (s *Service) Notify(payload string) {
	if s == nil {
		return
	}
	id, err := strconv.ParseInt(payload, 10, 64)
	if err != nil {
		return
	}
	select {
	case s.queue <- id:
	default:
		log.Printf("teams: queue full, not sending notification %d", id)
	}
}

// Run sends queued messages until ctx ends.
func (s *Service) Run(ctx context.Context) {
	if s == nil {
		return
	}
	for range senders {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case id := <-s.queue:
					s.send(ctx, id)
				}
			}
		}()
	}
}

// send tells Teams of one notification, unless it was read, its user has no webhook, or Teams was told of it already:
// a grouped notification is told again only after regroupInterval.
func (s *Service) send(ctx context.Context, id int64) {
	q := db.New(s.db)
	d, err := q.GetTeamsDelivery(ctx, id)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Printf("teams: could not load notification %d: %v", id, err)
		}
		return
	}
	if len(d.TeamsWebhook) == 0 || !d.IsActive || d.ReadAt.Valid {
		return
	}
	if d.TeamsSentAt.Valid {
		grouped := d.Kind == notifications.KindPersonChanged || d.Kind == notifications.KindClusterChanged
		if !grouped || time.Since(d.TeamsSentAt.Time) < regroupInterval {
			return
		}
	}
	if err := s.deliver(ctx, d.UserID, d.TeamsWebhook, s.text(d.Kind)); err != nil {
		return
	}
	if err := q.MarkTeamsSent(ctx, id); err != nil {
		log.Printf("teams: could not mark notification %d sent: %v", id, err)
	}
}

// deliver posts a message and records a failure on the user, or clears an earlier one.
func (s *Service) deliver(ctx context.Context, userID int64, sealed []byte, title string) error {
	q := db.New(s.db)
	webhook, err := s.open(sealed)
	if err == nil {
		err = s.checkURL(webhook)
	}
	if err == nil {
		err = s.post(ctx, webhook, title)
	}
	if err != nil {
		log.Printf("teams: could not tell user %d: %v", userID, err)
		if recErr := q.RecordTeamsFailure(ctx, db.RecordTeamsFailureParams{ID: userID, TeamsLastError: sql.NullString{String: err.Error(), Valid: true}}); recErr != nil {
			log.Printf("teams: could not record the failure for user %d: %v", userID, recErr)
		}
		return err
	}
	if err := q.ClearTeamsFailure(ctx, userID); err != nil {
		log.Printf("teams: could not clear the failure for user %d: %v", userID, err)
	}
	return nil
}

// post sends the Adaptive Card. Its errors never carry the URL, which is a credential.
func (s *Service) post(ctx context.Context, webhook, title string) error {
	body, err := json.Marshal(card(title, s.text("body"), s.text("open"), s.appURL))
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhook, bytes.NewReader(body))
	if err != nil {
		return ErrInvalidURL
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return fmt.Errorf("the webhook could not be reached: %v", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("the webhook answered %s", resp.Status)
	}
	return nil
}

// card is the Adaptive Card Power Automate's webhook trigger takes at its root.
func card(title, text, actionTitle, appURL string) map[string]any {
	c := map[string]any{
		"$schema": "http://adaptivecards.io/schemas/adaptive-card.json",
		"type":    "AdaptiveCard",
		"version": "1.4",
		"body": []map[string]any{
			{"type": "TextBlock", "size": "Medium", "weight": "Bolder", "text": title, "wrap": true},
			{"type": "TextBlock", "text": text, "wrap": true},
		},
	}
	if appURL != "" {
		c["actions"] = []map[string]any{{"type": "Action.OpenURL", "title": actionTitle, "url": appURL + "/?notifications=1"}}
	}
	return c
}

var texts = map[string]map[string]string{
	"pt-BR": {
		notifications.KindAnalysisAssigned: "TrackID: uma análise foi atribuída a você",
		notifications.KindPersonChanged:    "TrackID: uma pessoa que você segue tem novidades",
		notifications.KindClusterChanged:   "TrackID: um cluster que você segue tem novidades",
		"other":                            "TrackID: você tem uma nova notificação",
		"test":                             "TrackID: mensagem de teste",
		"body":                             "Abra o TrackID para ver os detalhes.",
		"open":                             "Abrir o TrackID",
	},
	"en": {
		notifications.KindAnalysisAssigned: "TrackID: an analysis was assigned to you",
		notifications.KindPersonChanged:    "TrackID: a person you follow has changes",
		notifications.KindClusterChanged:   "TrackID: a cluster you follow has changes",
		"other":                            "TrackID: you have a new notification",
		"test":                             "TrackID: test message",
		"body":                             "Open TrackID to see the details.",
		"open":                             "Open TrackID",
	},
}

func (s *Service) text(key string) string {
	lang := texts["pt-BR"]
	if s.language == "en" {
		lang = texts["en"]
	}
	if t, ok := lang[key]; ok {
		return t
	}
	return lang["other"]
}
