package ask

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestIsLocalEndpoint(t *testing.T) {
	for endpoint, want := range map[string]bool{
		"http://127.0.0.1:8090/v1":      true,
		"http://localhost:11434/v1":     true,
		"http://10.1.2.3/v1":            true,
		"http://192.168.0.9:8000/v1":    true,
		"http://strata:8080/v1":         true,
		"https://api.anthropic.com/v1":  false,
		"https://api.openai.com/v1":     false,
		"https://strata.example.org/v1": false,
		"http://8.8.8.8/v1":             false,
		"not a url":                     false,
		"":                              false,
	} {
		if got := isLocalEndpoint(endpoint); got != want {
			t.Errorf("isLocalEndpoint(%q) = %t", endpoint, got)
		}
	}
}

func TestNewModel(t *testing.T) {
	if m, err := NewModel(Settings{}); m != nil || err != nil {
		t.Errorf("unconfigured: %v, %v", m, err)
	}
	if _, err := NewModel(Settings{Endpoint: "http://127.0.0.1/v1"}); err == nil {
		t.Error("an endpoint with no model is accepted")
	}
	if _, err := NewModel(Settings{Endpoint: "http://127.0.0.1/v1", Model: "m", ExtraBody: "[1]"}); err == nil {
		t.Error("an ASK_EXTRA_BODY that is not an object is accepted")
	}
	m, err := NewModel(Settings{Endpoint: "https://api.example.org/v1/", Model: "m", ExtraBody: `{"max_tokens":100}`})
	if err != nil {
		t.Fatal(err)
	}
	if !m.External() || m.Host() != "api.example.org" || m.timeout != DefaultTimeout {
		t.Errorf("model: %+v", m)
	}
}

// fakeServer answers chat requests from a script, keeping what it was sent.
type fakeServer struct {
	*httptest.Server
	requests []map[string]any
	headers  []http.Header
}

func newFakeServer(t *testing.T, replies ...string) *fakeServer {
	t.Helper()
	f := &fakeServer{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path %q", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("request body: %v", err)
		}
		f.requests = append(f.requests, body)
		f.headers = append(f.headers, r.Header.Clone())
		if len(f.requests) > len(replies) {
			t.Errorf("request %d has no scripted reply", len(f.requests))
			http.Error(w, "no reply", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(replies[len(f.requests)-1]))
	}))
	t.Cleanup(f.Close)
	return f
}

// reply is a chat response with the final answer.
func reply(text string) string {
	content, _ := json.Marshal(text)
	return `{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":` + string(content) + `}}],
		"usage":{"prompt_tokens":10,"completion_tokens":5}}`
}

// callTool is a chat response calling one tool, with the null content servers send.
func callTool(id, name, arguments string) string {
	args, _ := json.Marshal(arguments)
	return `{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","content":null,"tool_calls":[
		{"id":"` + id + `","type":"function","function":{"name":"` + name + `","arguments":` + string(args) + `}}]}}],
		"usage":{"prompt_tokens":10,"completion_tokens":5}}`
}

func testModel(t *testing.T, f *fakeServer, s Settings) *Model {
	t.Helper()
	s.Endpoint, s.Model = f.URL+"/v1", "test-model"
	m, err := NewModel(s)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// TestConversationWire: the requests are the OpenAI protocol's, whatever the server, and the
// configuration decides the key and the extra fields.
func TestConversationWire(t *testing.T) {
	f := newFakeServer(t, callTool("call-1", "count_occurrences", `{"group_by":"year"}`), reply("<think>hmm</think>Forty."))
	m := testModel(t, f, Settings{APIKey: "secret", ExtraBody: `{"max_tokens":100,"model":"overridden"}`})
	c := m.start("system text", []Tool{{Name: "count_occurrences", Description: "d", Schema: map[string]any{}}},
		[]Message{{Role: "user", Text: "before"}, {Role: "assistant", Text: "after"}}, "how many?")

	first, err := c.next(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.calls) != 1 || first.calls[0].ID != "call-1" || first.calls[0].Arguments != `{"group_by":"year"}` {
		t.Fatalf("first turn: %+v", first)
	}
	second, err := c.next(context.Background(), []toolResult{{CallID: "call-1", Content: `{"total":40}`}})
	if err != nil {
		t.Fatal(err)
	}
	if second.text != "Forty." || len(second.calls) != 0 {
		t.Errorf("second turn: %+v", second)
	}

	if got := f.headers[0].Get("Authorization"); got != "Bearer secret" {
		t.Errorf("Authorization %q", got)
	}
	body := f.requests[0]
	if body["model"] != "test-model" || body["max_tokens"] != float64(100) || body["tool_choice"] != "auto" {
		t.Errorf("request: %v", body)
	}
	roles := []string{}
	for _, message := range f.requests[1]["messages"].([]any) {
		roles = append(roles, message.(map[string]any)["role"].(string))
	}
	if !reflect.DeepEqual(roles, []string{"system", "user", "assistant", "user", "assistant", "tool"}) {
		t.Errorf("roles %v", roles)
	}
	messages := f.requests[1]["messages"].([]any)
	if call := messages[4].(map[string]any); call["content"] != nil || len(call["tool_calls"].([]any)) != 1 {
		t.Errorf("the assistant's call went back as %v", call)
	}
	if result := messages[5].(map[string]any); result["tool_call_id"] != "call-1" || result["content"] != `{"total":40}` {
		t.Errorf("tool result: %v", result)
	}
}

func TestConversationErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		reply string
		want  error
	}{
		"truncated": {`{"choices":[{"finish_reason":"length","message":{"content":"par"}}]}`, ErrTruncated},
		"refused":   {`{"choices":[{"finish_reason":"content_filter","message":{"content":""}}]}`, ErrRefused},
	} {
		t.Run(name, func(t *testing.T) {
			m := testModel(t, newFakeServer(t, tc.reply), Settings{})
			if _, err := m.start("s", nil, nil, "q").next(context.Background(), nil); !errors.Is(err, tc.want) {
				t.Errorf("got %v", err)
			}
		})
	}
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad key", http.StatusUnauthorized)
	}))
	defer bad.Close()
	m, err := NewModel(Settings{Endpoint: bad.URL, Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.start("s", nil, nil, "q").next(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "bad key") {
		t.Errorf("a server's refusal reads %v", err)
	}
}

func TestUnverifiedNumbers(t *testing.T) {
	sources := []string{
		"quantos roubos em 2024?",
		`{"from":"2024-01-01","to":"2024-12-31"}`,
		`{"total":1234,"rows":[{"key":"2024-03","count":40,"sharePercent":12.5}]}`,
	}
	sources = append(sources, `{"score":688.9749,"threshold":60}`)
	for answer, want := range map[string][]string{
		"Escore 688,97 contra 60.":                    {},
		"Escore 688.97 contra 60.":                    {},
		"Escore 688,5 contra 60.":                     {"688,5"},
		"Escore 689 contra 60.":                       {"689"},
		"Foram 1.234 roubos em 2024.":                 {},
		"Foram 1,234 roubos.":                         {},
		"Em março, 40 ocorrências (12,5%).":           {},
		"De 01/01 a 31/12 de 2024 houve 1234.":        {},
		"Foram 1.235 roubos.":                         {"1.235"},
		"São 40, ou seja 13% do total, e 999 outros.": {"13", "999"},
		"1. Roubo: 40\n2. Furto: 40":                  {},
		"Sem números.":                                {},
		"999 e de novo 999":                           {"999"},
	} {
		if got := unverifiedNumbers(answer, sources); !reflect.DeepEqual(got, want) {
			t.Errorf("%q: got %v, want %v", answer, got, want)
		}
	}
}
