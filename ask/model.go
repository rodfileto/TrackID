// Package ask is the AI mode (docs/ask-plan.md): an intelligence analyst asks a question about the
// criminal scenario in plain language and a language model answers it. The model never writes SQL
// and never sees the database: it chooses among a fixed catalog of read-only tools (tools.go), the
// tools run parameterized queries, and every count in an answer comes from them. Numbers in the
// answer that the tools did not return are reported (verify.go), and each question is recorded in
// ask_queries.
//
// The model is whatever speaks the OpenAI chat protocol with tool calling at ASK_ENDPOINT: a local
// server such as Strata or Ollama, or another provider. Only the configuration tells them apart.
package ask

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultTimeout bounds one question, all of its turns together. A local model on modest hardware
// writes a few dozen tokens a second and a question can take several turns.
const DefaultTimeout = 5 * time.Minute

// ErrRefused is a model declining to answer; ErrTruncated one running out of output tokens.
var (
	ErrRefused   = errors.New("the model declined to answer")
	ErrTruncated = errors.New("the model's answer was cut off")
)

// Settings are the AI mode's configuration (internal/config).
type Settings struct {
	// Endpoint is the API's base URL, without "/chat/completions"; Model names the model. Both
	// empty means no AI mode.
	Endpoint string
	Model    string
	// APIKey is sent as a Bearer token; empty sends none.
	APIKey string
	// ExtraBody is a JSON object of request fields a server wants, such as temperature or
	// max_tokens: servers disagree on which they require and which they reject.
	ExtraBody string
	// Internal says the server is inside the institute although its name has dots (a host such as
	// llm.example.gov.br), which the name alone cannot show. Without it the address decides.
	Internal bool
	// Timeout bounds one question; zero means DefaultTimeout.
	Timeout time.Duration
}

// Model is a language model behind an OpenAI-compatible chat endpoint.
type Model struct {
	endpoint string
	name     string
	apiKey   string
	extra    map[string]any
	internal bool
	timeout  time.Duration
	http     *http.Client
}

// NewModel builds the configured model. It returns nil and no error when none is configured.
func NewModel(s Settings) (*Model, error) {
	if s.Endpoint == "" && s.Model == "" {
		return nil, nil
	}
	if s.Endpoint == "" || s.Model == "" {
		return nil, errors.New("the AI mode needs both an endpoint and a model")
	}
	m := &Model{
		endpoint: strings.TrimRight(s.Endpoint, "/"),
		name:     s.Model,
		apiKey:   s.APIKey,
		internal: s.Internal,
		timeout:  s.Timeout,
		// The context each question gets bounds its requests.
		http: &http.Client{},
	}
	if m.timeout <= 0 {
		m.timeout = DefaultTimeout
	}
	if strings.TrimSpace(s.ExtraBody) != "" {
		if err := json.Unmarshal([]byte(s.ExtraBody), &m.extra); err != nil {
			return nil, fmt.Errorf("the extra request fields are not a JSON object: %w", err)
		}
	}
	return m, nil
}

// Name is the model's name.
func (m *Model) Name() string { return m.name }

// Host is the host the model's server runs on, as recorded in ask_queries.
func (m *Model) Host() string {
	if u, err := url.Parse(m.endpoint); err == nil {
		return u.Host
	}
	return m.endpoint
}

// External says the server is outside the institute, so what the model reads leaves the machine.
// Such a model only ever reads synthetic data (Service.run).
func (m *Model) External() bool { return !m.internal && !isLocalEndpoint(m.endpoint) }

// isLocalEndpoint says whether an endpoint is inside the institute: a loopback or private address,
// or a host name with no dots. Anything else, or anything unreadable, counts as outside, so the
// guard on real data errs towards refusing.
func isLocalEndpoint(endpoint string) bool {
	u, err := url.Parse(endpoint)
	if err != nil || u.Hostname() == "" {
		return false
	}
	host := u.Hostname()
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback() || ip.IsPrivate()
	}
	return !strings.Contains(host, ".")
}

// Message is an earlier turn of the conversation, as text. Role is "user" or "assistant".
type Message struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

// Tool describes one tool to the model. Schema is the JSON Schema's "properties".
type Tool struct {
	Name        string
	Description string
	Schema      map[string]any
}

// toolCall is the model asking to run a tool. Arguments is the JSON it wrote, not yet validated.
type toolCall struct {
	ID        string
	Name      string
	Arguments string
}

// toolResult answers one toolCall. An error tells the model what was wrong, so it can correct
// itself.
type toolResult struct {
	CallID  string
	Content string
	IsError bool
}

// turn is one reply of the model: calls to run or, with no calls, the final answer in text.
type turn struct {
	text  string
	calls []toolCall
	usage Usage
}

// Usage counts tokens, added up over a question's turns.
type Usage struct {
	InputTokens  int64 `json:"inputTokens"`
	OutputTokens int64 `json:"outputTokens"`
}

// conversation is one question's exchange with the model.
type conversation struct {
	model    *Model
	messages []chatMessage
	tools    []chatTool
}

func (m *Model) start(system string, tools []Tool, history []Message, question string) *conversation {
	messages := []chatMessage{{Role: "system", Content: &system}}
	for _, h := range history {
		text := h.Text
		messages = append(messages, chatMessage{Role: h.Role, Content: &text})
	}
	messages = append(messages, chatMessage{Role: "user", Content: &question})
	defs := make([]chatTool, 0, len(tools))
	for _, t := range tools {
		defs = append(defs, chatTool{Type: "function", Function: chatFunction{
			Name: t.Name, Description: t.Description,
			Parameters: map[string]any{"type": "object", "properties": t.Schema},
		}})
	}
	return &conversation{model: m, messages: messages, tools: defs}
}

// next sends the results of the tools the model last called (none on the first call) and reads its
// next turn.
func (c *conversation) next(ctx context.Context, results []toolResult) (turn, error) {
	for _, r := range results {
		// The tool role has no error flag, so a failure is said in the content.
		content := r.Content
		if r.IsError {
			content = "ERROR: " + content
		}
		c.messages = append(c.messages, chatMessage{Role: "tool", ToolCallID: r.CallID, Content: &content})
	}
	payload, err := c.model.body(c.messages, c.tools)
	if err != nil {
		return turn{}, fmt.Errorf("ask: marshal chat request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.model.endpoint+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return turn{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.model.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.model.apiKey)
	}
	resp, err := c.model.http.Do(req)
	if err != nil {
		return turn{}, fmt.Errorf("ask: model request: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return turn{}, fmt.Errorf("ask: read model response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return turn{}, fmt.Errorf("ask: model %s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}
	var out chatResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return turn{}, fmt.Errorf("ask: parse model response: %w", err)
	}
	if out.Error != nil && out.Error.Message != "" {
		return turn{}, fmt.Errorf("ask: model error: %s", out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return turn{}, errors.New("ask: model returned no answer")
	}
	choice := out.Choices[0]
	switch choice.FinishReason {
	case "length":
		return turn{}, ErrTruncated
	case "content_filter", "refusal":
		return turn{}, ErrRefused
	}
	// Only what the protocol defines goes back: servers add fields of their own to a reply.
	sent := chatMessage{Role: "assistant", ToolCalls: choice.Message.ToolCalls}
	if choice.Message.Content != nil && *choice.Message.Content != "" {
		sent.Content = choice.Message.Content
	}
	c.messages = append(c.messages, sent)
	t := turn{}
	if choice.Message.Content != nil {
		t.text = stripThinking(*choice.Message.Content)
	}
	if out.Usage != nil {
		t.usage = Usage{InputTokens: out.Usage.PromptTokens, OutputTokens: out.Usage.CompletionTokens}
	}
	for _, call := range choice.Message.ToolCalls {
		t.calls = append(t.calls, toolCall{ID: call.ID, Name: call.Function.Name, Arguments: call.Function.Arguments})
	}
	return t, nil
}

// body is the request as JSON with ASK_EXTRA_BODY's fields added. Those never replace what the
// protocol exchange itself sets.
func (m *Model) body(messages []chatMessage, tools []chatTool) ([]byte, error) {
	request := map[string]any{"model": m.name, "messages": messages, "tools": tools, "tool_choice": "auto"}
	for k, v := range m.extra {
		if _, set := request[k]; !set {
			request[k] = v
		}
	}
	return json.Marshal(request)
}

// stripThinking drops the <think>...</think> block some servers return inside the answer.
func stripThinking(content string) string {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "<think>") {
		if end := strings.Index(content, "</think>"); end >= 0 {
			content = strings.TrimSpace(content[end+len("</think>"):])
		}
	}
	return content
}

type chatMessage struct {
	Role string `json:"role"`
	// Content is null on an assistant turn that only calls tools.
	Content    *string        `json:"content"`
	ToolCalls  []chatToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type chatTool struct {
	Type     string       `json:"type"`
	Function chatFunction `json:"function"`
}

type chatFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type chatToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content   *string        `json:"content"`
			ToolCalls []chatToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int64 `json:"prompt_tokens"`
		CompletionTokens int64 `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}
