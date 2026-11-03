package assist

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIClientDraft(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("content type %q", ct)
		}
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.Model != "test-model" || req.ResponseFormat == nil || req.ResponseFormat.Type != "json_object" {
			t.Errorf("request fields: %+v", req)
		}
		reply := `{"authority":"37ª DP","caseType":"CRIMINAL","cases":[{"task":"FACIAL_REVIEW","description":"revisão"}],"unsure":["dueOn"],"reasoning":"sem prazo"}`
		json.NewEncoder(w).Encode(chatResponse{Choices: []struct {
			Message chatMessage `json:"message"`
		}{{Message: chatMessage{Role: "assistant", Content: reply}}}})
	}))

	client := NewOpenAIClient(server.URL+"/v1", "test-model", []string{"LETHAL"})
	d, err := client.Draft(context.Background(), []Document{{Name: "o.pdf", Text: "texto"}}, "")
	if err != nil {
		t.Fatal(err)
	}
	if d.Request.Input.Authority != "37ª DP" || d.Request.CaseType != "CRIMINAL" {
		t.Errorf("draft: %+v", d.Request)
	}
	if len(d.Request.Cases) != 1 || d.Request.Cases[0].Task != "FACIAL_REVIEW" {
		t.Errorf("cases: %+v", d.Request.Cases)
	}
	if !contains(d.Unsure, "dueOn") || d.Reasoning != "sem prazo" {
		t.Errorf("unsure/reasoning: %v %q", d.Unsure, d.Reasoning)
	}
}

func TestOpenAIClientServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("boom"))
	}))
	client := NewOpenAIClient(server.URL, "m", nil)
	if _, err := client.Draft(context.Background(), nil, ""); err == nil || !strings.Contains(err.Error(), "500") {
		t.Errorf("expected non-200 error, got %v", err)
	}
}

func TestSystemPromptListsEnums(t *testing.T) {
	p := systemPrompt([]string{"LETHAL", "THEFT"})
	for _, want := range []string{"LETHAL", "THEFT", "FACIAL_REVIEW", "FINGERPRINT_COMPARISON", "CRIMINAL", "PUBLIC_ROAD"} {
		if !strings.Contains(p, want) {
			t.Errorf("system prompt missing %q", want)
		}
	}
}

func TestUserPromptInstructions(t *testing.T) {
	got := userPrompt([]Document{{Name: "o.pdf", Text: "texto"}}, "  recebido em 2026-10-30  ")
	if !strings.HasPrefix(got, "## Operator's instructions\n\nrecebido em 2026-10-30\n\n") || !strings.Contains(got, "### o.pdf\ntexto") {
		t.Errorf("unexpected prompt:\n%s", got)
	}
	if got := userPrompt(nil, "só dados"); !strings.Contains(got, "only the operator's instructions") && !strings.Contains(got, "(none:") {
		t.Errorf("no-documents prompt missing marker:\n%s", got)
	}
	if got := userPrompt([]Document{{Name: "a"}}, ""); strings.Contains(got, "Operator") {
		t.Errorf("empty instructions should add no section:\n%s", got)
	}
}

func TestOpenAIClientSendsAPIKey(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{}"}}]}`))
	}))
	defer srv.Close()
	if _, err := NewOpenAIClientWithOptions(srv.URL, "m", nil, OpenAIOptions{APIKey: "k123"}).Draft(context.Background(), nil, ""); err != nil {
		t.Fatal(err)
	}
	if got != "Bearer k123" {
		t.Errorf("Authorization = %q", got)
	}
	if _, err := NewOpenAIClient(srv.URL, "m", nil).Draft(context.Background(), nil, ""); err != nil || got != "" {
		t.Errorf("no key should send no header, got %q (err %v)", got, err)
	}
}

func TestOpenAIClientDisablesThinkingAndReportsUsage(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"<think>hm</think>{\"authority\":\"37ª DP\"}"}}],` +
			`"usage":{"prompt_tokens":100,"completion_tokens":40,"completion_tokens_details":{"reasoning_tokens":3}}}`))
	}))
	defer srv.Close()
	d, err := NewOpenAIClient(srv.URL, "m", nil).Draft(context.Background(), nil, "")
	if err != nil {
		t.Fatal(err)
	}
	kw, _ := body["chat_template_kwargs"].(map[string]any)
	if kw["enable_thinking"] != false {
		t.Errorf("chat_template_kwargs = %v, want enable_thinking false", body["chat_template_kwargs"])
	}
	if d.Request.Input.Authority != "37ª DP" {
		t.Errorf("think block not stripped: %+v", d.Request.Input)
	}
	if d.Usage == nil || d.Usage.PromptTokens != 100 || d.Usage.CompletionTokens != 40 || d.Usage.ReasoningTokens != 3 {
		t.Errorf("usage = %+v", d.Usage)
	}
}
