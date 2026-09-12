package infobio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

const (
	consultaPath = "/ajax/ajax_load_consulta.php"
	detalhesPath = "/ajax/ajax_load_detalhes.php"
)

var rinPattern = regexp.MustCompile(`^([A-Za-z]*)([0-9]+)$`)

type SearchParams struct {
	NIF            string
	RIN            string
	Nome           string
	NomePai        string
	NomeMae        string
	DataNascimento string
}

type Entry map[string]any

type SearchResult struct {
	Entries []Entry `json:"entries"`
}

type RequestHook func(*http.Request)

type SearchService struct {
	BaseURL     string
	HTTPClient  *http.Client
	RequestHook RequestHook
}

func (service *SearchService) SearchHandler() http.HandlerFunc {
	return func(responseWriter http.ResponseWriter, request *http.Request) {
		params := SearchParams{
			NIF:            request.URL.Query().Get("nif"),
			RIN:            request.URL.Query().Get("rin"),
			Nome:           request.URL.Query().Get("nome"),
			NomePai:        request.URL.Query().Get("nome_pai"),
			NomeMae:        request.URL.Query().Get("nome_mae"),
			DataNascimento: request.URL.Query().Get("data_nascimento"),
		}
		result, err := service.Search(request.Context(), params)
		if err != nil {
			status := http.StatusBadGateway
			if strings.Contains(err.Error(), "provide one search field") || strings.Contains(err.Error(), "not configured") {
				status = http.StatusBadRequest
			}
			http.Error(responseWriter, err.Error(), status)
			return
		}
		responseWriter.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(responseWriter).Encode(result)
	}
}

func NewSearchService(baseURL string, client *http.Client, hook RequestHook) *SearchService {
	if client == nil {
		client = http.DefaultClient
	}
	return &SearchService{BaseURL: strings.TrimRight(baseURL, "/"), HTTPClient: client, RequestHook: hook}
}

func NormalizeRIN(value string) string {
	value = strings.TrimSpace(value)
	match := rinPattern.FindStringSubmatch(value)
	if match == nil {
		return value
	}
	padding := 20 - len(match[2])
	if padding < 0 {
		padding = 0
	}
	return match[1] + strings.Repeat("0", padding) + match[2]
}

func (service *SearchService) Search(ctx context.Context, params SearchParams) (SearchResult, error) {
	field, value, err := params.searchField()
	if err != nil {
		return SearchResult{}, err
	}
	if field == "rin" {
		value = NormalizeRIN(value)
	}

	response, err := service.post(ctx, consultaPath, url.Values{
		"nomePessoa":     {strings.TrimSpace(params.Nome)},
		"nomePai":        {strings.TrimSpace(params.NomePai)},
		"nomeMae":        {strings.TrimSpace(params.NomeMae)},
		"dataNascimento": {strings.TrimSpace(params.DataNascimento)},
		"rin":            {valueIf(field == "rin", value)},
		"numeroNIF":      {valueIf(field == "nif", value)},
	})
	if err != nil {
		return SearchResult{}, err
	}
	entries, err := decodeEntries(response)
	if err != nil {
		return SearchResult{}, err
	}
	if len(entries) == 0 || isNotFound(entries[0]) {
		return SearchResult{Entries: []Entry{}}, nil
	}

	for _, entry := range entries {
		if container := stringValue(entry, "container"); container != "" {
			service.mergeDetails(ctx, entry, container)
		}
	}

	if field == "nif" || field == "nome" {
		entries, err = service.expandContainers(ctx, entries)
		if err != nil {
			return SearchResult{}, err
		}
	}
	stripSQL(entries)
	return SearchResult{Entries: entries}, nil
}

func (params SearchParams) searchField() (string, string, error) {
	if strings.TrimSpace(params.NIF) != "" {
		return "nif", strings.TrimSpace(params.NIF), nil
	}
	if strings.TrimSpace(params.RIN) != "" {
		return "rin", strings.TrimSpace(params.RIN), nil
	}
	if strings.TrimSpace(params.Nome) != "" {
		return "nome", strings.TrimSpace(params.Nome), nil
	}
	return "", "", errors.New("provide one search field: nif, rin, or nome")
}

func (service *SearchService) post(ctx context.Context, path string, form url.Values) ([]byte, error) {
	if service.BaseURL == "" {
		return nil, errors.New("infobio base URL is not configured")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, service.BaseURL+path, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if service.RequestHook != nil {
		service.RequestHook(request)
	}
	response, err := service.HTTPClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("infobio request failed: HTTP %d", response.StatusCode)
	}
	return bytesTrimBOM(body), nil
}

func (service *SearchService) fetchContainer(ctx context.Context, container string) ([]Entry, error) {
	body, err := service.post(ctx, detalhesPath, url.Values{"container": {container}})
	if err != nil {
		return nil, err
	}
	return decodeEntries(body)
}

func (service *SearchService) mergeDetails(ctx context.Context, target Entry, container string) {
	entries, err := service.fetchContainer(ctx, container)
	if err == nil && len(entries) > 0 {
		for key, value := range entries[0] {
			target[key] = value
		}
	}
}

func (service *SearchService) expandContainers(ctx context.Context, entries []Entry) ([]Entry, error) {
	containers := map[string]bool{}
	seenNIFs := map[string]bool{}
	for _, entry := range entries {
		if container := stringValue(entry, "container"); container != "" {
			containers[container] = true
		}
		if nif := extractNIF(entry); nif != "" {
			seenNIFs[nif] = true
		}
	}
	containerNames := make([]string, 0, len(containers))
	for container := range containers {
		containerNames = append(containerNames, container)
	}
	sort.Strings(containerNames)

	expanded := append([]Entry{}, entries...)
	for _, container := range containerNames {
		siblings, err := service.fetchContainer(ctx, container)
		if err != nil {
			continue
		}
		for _, sibling := range siblings {
			nif := extractNIF(sibling)
			if nif != "" && !seenNIFs[nif] {
				seenNIFs[nif] = true
				expanded = append(expanded, sibling)
			}
		}
	}
	return expanded, nil
}

func decodeEntries(body []byte) ([]Entry, error) {
	if len(strings.TrimSpace(string(body))) == 0 {
		return []Entry{}, nil
	}
	var raw any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	if object, ok := raw.(map[string]any); ok {
		if len(object) == 0 {
			return []Entry{}, nil
		}
		return []Entry{object}, nil
	}
	array, ok := raw.([]any)
	if !ok {
		return nil, errors.New("infobio response must be an object or array")
	}
	entries := make([]Entry, 0, len(array))
	for _, item := range array {
		entry, ok := item.(map[string]any)
		if ok {
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

func extractNIF(entry Entry) string {
	for _, key := range []string{"NIF", "nif", "numeroNIF"} {
		if value := stringValue(entry, key); value != "" {
			return value
		}
	}
	return ""
}

func stringValue(entry Entry, key string) string {
	value, _ := entry[key].(string)
	return strings.TrimSpace(value)
}

// stripSQL removes InfoBio's echoed "sql" field from every entry before a result leaves this
// package. InfoBio includes the literal executed query (schema/table names, the search value
// interpolated as a quoted literal) in its responses, which is an information leak on InfoBio's
// side that TrackID should not re-expose to its own API clients or UI.
func stripSQL(entries []Entry) {
	for _, entry := range entries {
		delete(entry, "sql")
	}
}

func isNotFound(entry Entry) bool {
	value, ok := entry["id"].(float64)
	return ok && value == 0 && entry["sql"] != nil
}

func valueIf(condition bool, value string) string {
	if condition {
		return value
	}
	return ""
}

func bytesTrimBOM(body []byte) []byte {
	return []byte(strings.TrimPrefix(string(body), "\ufeff"))
}
