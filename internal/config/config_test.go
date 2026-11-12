package config

import (
	"strings"
	"testing"

	"github.com/rodfileto/trackid/llm"
)

func productionConfig() Config {
	return Config{
		Environment: EnvironmentProduction,
		DatabaseURL: "postgres://x", RedisURL: "redis://x",
		S3Endpoint: "http://s3", S3AccessKey: "a", S3SecretKey: "s", S3Bucket: "b",
		JWTSecret: strings.Repeat("k", MinJWTSecretBytes), AppURL: "https://trackid.example",
	}
}

func TestCheckProductionAcceptsAComplete(t *testing.T) {
	if err := productionConfig().CheckProduction(ProcessAPI); err != nil {
		t.Fatal(err)
	}
}

func TestCheckProductionNamesEveryProblem(t *testing.T) {
	c := productionConfig()
	c.S3Bucket, c.JWTSecret, c.AppURL = "", "short", "http://trackid.example"
	err := c.CheckProduction(ProcessAPI)
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"S3_BUCKET", "JWT_SECRET", "APP_URL"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error doesn't name %s: %v", want, err)
		}
	}
}

func TestCheckProductionAsksTheWorkerOnlyWhatItUses(t *testing.T) {
	c := productionConfig()
	c.JWTSecret, c.AppURL = "", ""
	if err := c.CheckProduction(ProcessWorker); err != nil {
		t.Fatalf("the worker signs no tokens and sends no links: %v", err)
	}
}

func TestCheckProductionIsSilentInDevelopment(t *testing.T) {
	if err := (Config{Environment: EnvironmentDevelopment}).CheckProduction(ProcessAPI); err != nil {
		t.Fatal(err)
	}
}

func TestParseTrustedProxies(t *testing.T) {
	got, err := parseTrustedProxies(" 10.0.0.0/8, 172.18.0.5 ,")
	if err != nil || len(got) != 2 || got[0] != "10.0.0.0/8" || got[1] != "172.18.0.5" {
		t.Fatalf("got %v, %v", got, err)
	}
	if _, err := parseTrustedProxies("nginx"); err == nil {
		t.Fatal("a host name was accepted")
	}
}

func TestBuildModels(t *testing.T) {
	clear := func() {
		for _, k := range []string{"ASSIST_ENDPOINT", "ASSIST_MODEL", "ASSIST_API_KEY", "ASSIST_INTERNAL", "ASK_ENDPOINT", "ASK_MODEL", "ASK_API_KEY", "ASK_EXTRA_BODY"} {
			t.Setenv(k, "")
		}
	}

	clear()
	if _, ok := buildModels().For(llm.Ask); ok {
		t.Error("no ASSIST_ENDPOINT, and a model resolved")
	}

	// The platform's host alone: every use follows the table, whatever the old /v1 suffix.
	clear()
	t.Setenv("ASSIST_ENDPOINT", "https://llm.example.gov.br/v1")
	t.Setenv("ASSIST_API_KEY", "k")
	t.Setenv("ASSIST_INTERNAL", "true")
	c := buildModels()
	if r, _ := c.For(llm.Ask); r.BaseURL != "https://llm.example.gov.br/v1" || r.Model != "llm" || r.APIKey != "k" || !r.Internal {
		t.Errorf("ask: %+v", r)
	}
	if r, _ := c.For(llm.OCR); r.BaseURL != "https://llm.example.gov.br/aiocr" || r.Model != "ocr" {
		t.Errorf("ocr: %+v", r)
	}

	// ASSIST_MODEL points the AI mode and drafting at a server that is not the platform's, as given.
	clear()
	t.Setenv("ASSIST_ENDPOINT", "http://127.0.0.1:11434/v1")
	t.Setenv("ASSIST_MODEL", "qwen3")
	c = buildModels()
	for _, use := range []llm.Use{llm.Ask, llm.AssistDraft} {
		if r, _ := c.For(use); r.BaseURL != "http://127.0.0.1:11434/v1" || r.Model != "qwen3" {
			t.Errorf("%s: %+v", use, r)
		}
	}

	// ASK_* wins for the AI mode, takes its own key, and brings its extra fields.
	t.Setenv("ASK_ENDPOINT", "https://api.example.org/v1")
	t.Setenv("ASK_MODEL", "big")
	t.Setenv("ASK_API_KEY", "ask-key")
	t.Setenv("ASK_EXTRA_BODY", `{"max_tokens":16000}`)
	c = buildModels()
	if r, _ := c.For(llm.Ask); r.BaseURL != "https://api.example.org/v1" || r.Model != "big" || r.APIKey != "ask-key" || r.ExtraJSON() != `{"max_tokens":16000}` {
		t.Errorf("ask override: %+v", r)
	}
	if r, _ := c.For(llm.AssistDraft); r.Model != "qwen3" || r.APIKey != "" {
		t.Errorf("drafting changed: %+v", r)
	}
}
