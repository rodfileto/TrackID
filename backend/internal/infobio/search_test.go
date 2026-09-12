package infobio

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNormalizeRIN(t *testing.T) {
	cases := map[string]string{
		"R34717269":              "R00000000000034717269",
		"EB0134362":              "EB00000000000000134362",
		"R00000000000034717269":  "R00000000000034717269",
		"R123456789012345678901": "R123456789012345678901",
		"not-a-rin":              "not-a-rin",
	}
	for input, expected := range cases {
		if actual := NormalizeRIN(input); actual != expected {
			t.Errorf("NormalizeRIN(%q) = %q, want %q", input, actual, expected)
		}
	}
}

func TestSearchByNIFExpandsContainerSiblings(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if err := request.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if request.URL.Path == consultaPath {
			if got := request.Form.Get("numeroNIF"); got != "NIF-001" {
				t.Errorf("numeroNIF = %q", got)
			}
			if got := request.Form.Get("rin"); got != "" {
				t.Errorf("rin = %q", got)
			}
			fmt.Fprint(responseWriter, `[{"nif":"NIF-001","container":"C-001","nome":"Maria"}]`)
			return
		}
		if request.URL.Path == detalhesPath {
			if got := request.Form.Get("container"); got != "C-001" {
				t.Errorf("container = %q", got)
			}
			fmt.Fprint(responseWriter, `[{"nif":"NIF-001","container":"C-001","nome":"Maria"},{"nif":"NIF-002","container":"C-001","nome":"Ana"}]`)
			return
		}
		http.NotFound(responseWriter, request)
	}))
	defer server.Close()

	service := NewSearchService(server.URL, server.Client(), nil)
	result, err := service.Search(context.Background(), SearchParams{NIF: "NIF-001"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(result.Entries))
	}
	if got := stringValue(result.Entries[1], "nif"); got != "NIF-002" {
		t.Errorf("sibling NIF = %q", got)
	}
}

func TestSearchByRINNormalizesRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if err := request.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if got := request.Form.Get("rin"); got != "R00000000000034717269" {
			t.Errorf("rin = %q", got)
		}
		fmt.Fprint(responseWriter, `{"nif":"NIF-001"}`)
	}))
	defer server.Close()

	service := NewSearchService(server.URL, server.Client(), nil)
	result, err := service.Search(context.Background(), SearchParams{RIN: "R34717269"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(result.Entries))
	}
}

func TestSearchRequiresOneField(t *testing.T) {
	service := NewSearchService("http://unused", nil, nil)
	if _, err := service.Search(context.Background(), SearchParams{}); err == nil {
		t.Fatal("expected validation error")
	}
}
