package llm

import "testing"

func TestEveryUseHasARoute(t *testing.T) {
	c := New("https://llm.example.gov.br", "", false)
	for _, use := range Uses() {
		r, ok := c.For(use)
		if !ok || r.BaseURL == "" || r.Model == "" {
			t.Errorf("%s: not resolved: %+v", use, r)
		}
	}
	if len(Uses()) != len(routes) {
		t.Errorf("Uses lists %d, the table has %d", len(Uses()), len(routes))
	}
}

func TestPlatformLayout(t *testing.T) {
	c := New("https://llm.example.gov.br/", "key", true)
	for use, want := range map[Use][2]string{
		Ask:         {"https://llm.example.gov.br/v1", "llm"},
		AssistDraft: {"https://llm.example.gov.br/v1", "llm"},
		Light:       {"https://llm.example.gov.br/ai_llm_micro", "default"},
		OCR:         {"https://llm.example.gov.br/aiocr", "ocr"},
		Speech:      {"https://llm.example.gov.br/stt", "stt"},
	} {
		r, _ := c.For(use)
		if r.BaseURL != want[0] || r.Model != want[1] || r.APIKey != "key" || !r.Internal {
			t.Errorf("%s: got %+v, want %v", use, r, want)
		}
	}
}

func TestHostWithTheOldV1Suffix(t *testing.T) {
	r, _ := New("https://llm.example.gov.br/v1", "", false).For(Light)
	if r.BaseURL != "https://llm.example.gov.br/ai_llm_micro" {
		t.Errorf("got %s", r.BaseURL)
	}
}

func TestNoHostMeansNoModel(t *testing.T) {
	if _, ok := New("", "", false).For(Ask); ok {
		t.Error("a Catalog with no host resolved a Use")
	}
	if _, ok := New("https://x.gov.br", "", false).For(Use("nonexistent")); ok {
		t.Error("an unknown Use resolved")
	}
}

func TestOverrideWinsAndNeedsAllItsParts(t *testing.T) {
	c := New("https://llm.example.gov.br", "platform-key", false)
	if err := c.Override(Ask, Override{Endpoint: "http://127.0.0.1:11434/v1/", Model: "qwen3"}); err != nil {
		t.Fatal(err)
	}
	r, _ := c.For(Ask)
	if r.BaseURL != "http://127.0.0.1:11434/v1" || r.Model != "qwen3" || r.APIKey != "" {
		t.Errorf("override not used: %+v", r)
	}
	if other, _ := c.For(AssistDraft); other.Model != "llm" || other.APIKey != "platform-key" {
		t.Errorf("an override leaked to another use: %+v", other)
	}
	if c.Override(Ask, Override{Endpoint: "http://x"}) == nil || c.Override(Ask, Override{Model: "m"}) == nil {
		t.Error("a partial override was accepted")
	}
	if c.Override(Ask, Override{Endpoint: "not a url", Model: "m"}) == nil {
		t.Error("a bad endpoint was accepted")
	}
	if c.Override(Use("nonexistent"), Override{Endpoint: "http://x", Model: "m"}) == nil {
		t.Error("an unknown use was accepted")
	}
}

func TestExtraJSON(t *testing.T) {
	r, _ := New("https://x.gov.br", "", false).For(Ask)
	if got := r.ExtraJSON(); got != `{"chat_template_kwargs":{"enable_thinking":false},"temperature":0}` {
		t.Errorf("got %s", got)
	}
	if r, _ := New("https://x.gov.br", "", false).For(OCR); r.ExtraJSON() != "" {
		t.Errorf("OCR has extra fields: %s", r.ExtraJSON())
	}
}
