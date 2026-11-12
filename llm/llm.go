// Package llm is the table that says which model each feature talks to. A feature names its Use;
// the table gives the endpoint, the model alias and the request fields that feature wants, so the
// choice is made once, in code, and is the same on every machine. Nothing here is read from the
// environment but the server's address and key.
//
// To give a new feature a model, add a Use constant and its row to routes, and call
// Catalog.For(that Use) where the feature builds its client.
//
// The table follows the organisation's platform (OpenShift AI, vLLM, OpenAI-compatible): one
// host, one path prefix per model. It prefers the platform's public aliases ("llm", "ocr", "stt"
// and "default"), which the platform keeps stable while it swaps the model behind them, over the
// real model names.
package llm

import (
	"encoding/json"
	"errors"
	"net/url"
	"strings"
)

// Use is a feature that calls a model.
type Use string

const (
	// Ask is the AI mode (package ask): a multi-turn loop of tool calls over a fixed catalog, so it
	// needs tool calling and room for the tool results.
	Ask Use = "ask"
	// AssistDraft drafts a registration from a request's files (package assist): long documents in,
	// one JSON object out.
	AssistDraft Use = "assist.draft"
	// Light is for short, high-volume tasks that need no tools and little context: classifying,
	// summarising a paragraph.
	Light Use = "light"
	// OCR reads text from an image (a chat endpoint taking image_url parts).
	OCR Use = "ocr"
	// Speech transcribes audio (multipart audio/transcriptions, not chat).
	Speech Use = "stt"
)

// Route is where a Use goes: a path prefix under the host, the model alias to send, and request
// fields that endpoint wants. Prefix is the base URL the client appends its own path to
// ("/chat/completions", "/audio/transcriptions").
type Route struct {
	Prefix string
	Model  string
	// Extra are request fields added to the body, never replacing the ones the feature sets.
	Extra map[string]any
}

// thinkingOff turns off the reasoning a Qwen model writes before it answers. In a loop of tool
// calls that costs seconds on every turn, and the answers are checked against the tools anyway.
var thinkingOff = map[string]any{"enable_thinking": false}

// routes is the table. The main model ("llm", 262k context, tool calling) takes anything long or
// that uses tools; the micro model (65k, no tool calling) the short work; the others are the
// platform's OCR and speech models.
var routes = map[Use]Route{
	Ask:         {Prefix: "/v1", Model: "llm", Extra: map[string]any{"temperature": 0, "chat_template_kwargs": thinkingOff}},
	AssistDraft: {Prefix: "/v1", Model: "llm"},
	Light:       {Prefix: "/ai_llm_micro", Model: "default", Extra: map[string]any{"chat_template_kwargs": thinkingOff}},
	OCR:         {Prefix: "/aiocr", Model: "ocr"},
	Speech:      {Prefix: "/stt", Model: "stt"},
}

// Uses lists every Use in the table.
func Uses() []Use {
	return []Use{Ask, AssistDraft, Light, OCR, Speech}
}

// Resolved is a Use ready to call.
type Resolved struct {
	Use Use
	// BaseURL is the endpoint's base, with no trailing slash and without the path the client adds.
	BaseURL string
	// Model is the alias to send as "model".
	Model  string
	APIKey string
	// Extra are request fields to add to the body.
	Extra map[string]any
	// Internal says the server is inside the institute, which the host's name alone may not show.
	Internal bool
}

// ExtraJSON is Extra as a JSON object, or "" when there is none.
func (r Resolved) ExtraJSON() string {
	if len(r.Extra) == 0 {
		return ""
	}
	b, _ := json.Marshal(r.Extra)
	return string(b)
}

// Override sends one Use to a server that is not the platform's -- Ollama, a local relay -- with
// its own endpoint (verbatim, so with its "/v1"), model and extra fields.
type Override struct {
	Endpoint string
	Model    string
	APIKey   string
	Extra    map[string]any
}

// Catalog resolves each Use against one host.
type Catalog struct {
	host      string
	apiKey    string
	internal  bool
	overrides map[Use]Override
}

// New builds a Catalog for the platform's host, such as "https://llm.mg.pf.gov.br". A trailing
// "/" or "/v1" is dropped, so the old style of setting, that ended in "/v1", still works. internal
// says the host is inside the institute even when its name has dots.
func New(host, apiKey string, internal bool) *Catalog {
	host = strings.TrimRight(strings.TrimSpace(host), "/")
	host = strings.TrimSuffix(host, "/v1")
	return &Catalog{host: host, apiKey: apiKey, internal: internal, overrides: map[Use]Override{}}
}

// Override sends use to another server instead of the platform's. A zero Endpoint or Model is an
// error: an override is all of its parts or nothing.
func (c *Catalog) Override(use Use, o Override) error {
	if _, ok := routes[use]; !ok {
		return errors.New("llm: unknown use " + string(use))
	}
	if o.Endpoint == "" || o.Model == "" {
		return errors.New("llm: an override for " + string(use) + " needs both an endpoint and a model")
	}
	if _, err := url.ParseRequestURI(o.Endpoint); err != nil {
		return errors.New("llm: the override endpoint for " + string(use) + " is not a URL")
	}
	c.overrides[use] = o
	return nil
}

// For resolves use. It reports false when the Use has no model: no host is set (and no override),
// or the Use is not in the table. A caller treats that as the feature being off. A nil Catalog
// resolves nothing.
func (c *Catalog) For(use Use) (Resolved, bool) {
	if c == nil {
		return Resolved{}, false
	}
	if o, ok := c.overrides[use]; ok {
		return Resolved{Use: use, BaseURL: strings.TrimRight(o.Endpoint, "/"), Model: o.Model, APIKey: o.APIKey, Extra: o.Extra, Internal: c.internal}, true
	}
	route, ok := routes[use]
	if !ok || c.host == "" {
		return Resolved{}, false
	}
	return Resolved{Use: use, BaseURL: c.host + route.Prefix, Model: route.Model, APIKey: c.apiKey, Extra: route.Extra, Internal: c.internal}, true
}
