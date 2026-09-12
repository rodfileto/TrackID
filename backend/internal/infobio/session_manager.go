package infobio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	loginPath        = "/php/login.php"
	checkSessionPath = "/ajax/ajax_checksession.php"
)

var ErrSessionCheckFailed = errors.New("infobio session check failed")

type AuthRequiredError struct {
	Expired bool
	Detail  string
}

func (err *AuthRequiredError) Error() string {
	if err.Detail != "" {
		return err.Detail
	}
	if err.Expired {
		return "Sessao InfoBio expirada"
	}
	return "Sessao InfoBio nao iniciada"
}

type SessionManager struct {
	BaseURL    string
	HTTPClient *http.Client
	Store      SessionStore
	TTL        time.Duration
}

func NewSessionManager(baseURL string, client *http.Client, store SessionStore) *SessionManager {
	if client == nil {
		client = http.DefaultClient
	}
	if store == nil {
		store = NewMemorySessionStore()
	}
	return &SessionManager{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		HTTPClient: client,
		Store:      store,
		TTL:        2 * time.Hour,
	}
}

func (manager *SessionManager) CreateUserSession(ctx context.Context, userID int64, username, password string) error {
	if strings.TrimSpace(username) == "" || strings.TrimSpace(password) == "" {
		return errors.New("username and password are required")
	}
	cookies, err := manager.authenticate(ctx, username, password)
	if err != nil {
		return err
	}
	if len(cookies) == 0 {
		return errors.New("autenticacao nao retornou cookies de sessao")
	}
	return manager.Store.Save(ctx, userID, StoredSession{Cookies: cookies, Username: username}, manager.TTL)
}

func (manager *SessionManager) SessionStatus(ctx context.Context, userID int64) (bool, error) {
	session, ok, err := manager.Store.Load(ctx, userID)
	if err != nil || !ok {
		return false, err
	}
	alive, err := manager.keepAlive(ctx, session.Cookies)
	if err != nil {
		return false, nil
	}
	if !alive {
		_ = manager.Store.Delete(ctx, userID)
		return false, nil
	}
	return true, nil
}

func (manager *SessionManager) InvalidateUserSession(ctx context.Context, userID int64) error {
	return manager.Store.Delete(ctx, userID)
}

func (manager *SessionManager) ValidatedCookies(ctx context.Context, userID int64) (map[string]string, error) {
	session, ok, err := manager.Store.Load(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, &AuthRequiredError{Expired: false, Detail: "Sessao InfoBio nao iniciada. Informe sua senha para continuar."}
	}
	alive, err := manager.keepAlive(ctx, session.Cookies)
	if err != nil {
		// Network errors should not invalidate the cached session.
		return session.Cookies, nil
	}
	if !alive {
		_ = manager.Store.Delete(ctx, userID)
		return nil, &AuthRequiredError{Expired: true, Detail: "Sessao InfoBio expirada. Informe sua senha novamente."}
	}
	return session.Cookies, nil
}

func (manager *SessionManager) HookFromCookies(cookies map[string]string) RequestHook {
	if len(cookies) == 0 {
		return nil
	}
	parts := make([]string, 0, len(cookies))
	for key, value := range cookies {
		parts = append(parts, key+"="+value)
	}
	sort.Strings(parts)
	cookieHeader := strings.Join(parts, "; ")
	return func(request *http.Request) {
		request.Header.Set("Cookie", cookieHeader)
	}
}

func (manager *SessionManager) keepAlive(ctx context.Context, cookies map[string]string) (bool, error) {
	if manager.BaseURL == "" {
		return false, errors.New("infobio base URL is not configured")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, manager.BaseURL+checkSessionPath, nil)
	if err != nil {
		return false, err
	}
	manager.addCookies(request, cookies)
	response, err := manager.HTTPClient.Do(request)
	if err != nil {
		return false, fmt.Errorf("%w: %v", ErrSessionCheckFailed, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return false, nil
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return false, fmt.Errorf("%w: %v", ErrSessionCheckFailed, err)
	}
	return strings.TrimSpace(string(body)) == "1", nil
}

func (manager *SessionManager) authenticate(ctx context.Context, username, password string) (map[string]string, error) {
	if manager.BaseURL == "" {
		return nil, errors.New("infobio base URL is not configured")
	}
	form := url.Values{
		"email":       {username},
		"salva_email": {"true"},
		"senha":       {password},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, manager.BaseURL+loginPath, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	client := *manager.HTTPClient
	client.Jar = jar
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("erro na autenticacao: HTTP %d", response.StatusCode)
	}
	cookies := map[string]string{}
	for _, cookie := range jar.Cookies(request.URL) {
		cookies[cookie.Name] = cookie.Value
	}
	for _, cookie := range response.Cookies() {
		cookies[cookie.Name] = cookie.Value
	}
	return cookies, nil
}

func (manager *SessionManager) addCookies(request *http.Request, cookies map[string]string) {
	for name, value := range cookies {
		request.AddCookie(&http.Cookie{Name: name, Value: value})
	}
}
