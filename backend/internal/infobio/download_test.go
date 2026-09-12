package infobio

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDownloadJPGUsesAlternativeNIFAfterRetries(t *testing.T) {
	primaryAttempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/images/NIF-PRIMARY.jpg":
			primaryAttempts++
			responseWriter.WriteHeader(http.StatusNotFound)
		case "/images/NIF-ALTERNATIVE.jpg":
			responseWriter.Header().Set("Content-Type", "image/jpeg")
			_, _ = responseWriter.Write([]byte("jpeg-bytes"))
		default:
			http.NotFound(responseWriter, request)
		}
	}))
	defer server.Close()

	service := NewDownloadService(server.URL+"/images", "", server.Client(), nil)
	service.RetryDelay = 0
	image, err := service.DownloadJPG(context.Background(), "NIF-PRIMARY", []string{"NIF-ALTERNATIVE"})
	if err != nil {
		t.Fatal(err)
	}
	if image.NIF != "NIF-ALTERNATIVE" || string(image.Content) != "jpeg-bytes" {
		t.Fatalf("unexpected downloaded image: %+v", image)
	}
	if primaryAttempts != 3 {
		t.Fatalf("primary attempts = %d, want 3", primaryAttempts)
	}
}

func TestDownloadJPGRejectsNonImageResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		responseWriter.Header().Set("Content-Type", "text/html")
		fmt.Fprint(responseWriter, "not an image")
	}))
	defer server.Close()

	service := NewDownloadService(server.URL, "", server.Client(), nil)
	service.MaxRetries = 1
	_, err := service.DownloadJPG(context.Background(), "NIF-001", nil)
	if err == nil || !strings.Contains(err.Error(), "no image available") {
		t.Fatalf("expected image validation error, got %v", err)
	}
}

func TestDownloadPDFFromImagesURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/images/NIF-001.pdf" {
			http.NotFound(responseWriter, request)
			return
		}
		responseWriter.Header().Set("Content-Type", "application/pdf")
		_, _ = responseWriter.Write([]byte("pdf-bytes"))
	}))
	defer server.Close()

	service := NewDownloadService(server.URL+"/images", "", server.Client(), nil)
	content, err := service.DownloadPDF(context.Background(), "NIF-001")
	if err != nil || string(content) != "pdf-bytes" {
		t.Fatalf("unexpected PDF result: %q, %v", content, err)
	}
}

func TestDownloadNISTResolvesRelativePath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/Nists/Exportados/NIF-001.nist" {
			http.NotFound(responseWriter, request)
			return
		}
		_, _ = responseWriter.Write([]byte("nist-bytes"))
	}))
	defer server.Close()

	service := NewDownloadService("", server.URL, server.Client(), nil)
	content, err := service.DownloadNIST(context.Background(), "NIF-001", "/Nists/Exportados/NIF-001.nist")
	if err != nil || string(content) != "nist-bytes" {
		t.Fatalf("unexpected NIST result: %q, %v", content, err)
	}
}

func TestDownloadNISTUsesPythonFallbackPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/Nists/NIF-002.nist" {
			http.NotFound(responseWriter, request)
			return
		}
		_, _ = responseWriter.Write([]byte("nist-fallback"))
	}))
	defer server.Close()

	service := NewDownloadService("", server.URL, server.Client(), nil)
	content, err := service.DownloadNIST(context.Background(), "NIF-002", "")
	if err != nil || string(content) != "nist-fallback" {
		t.Fatalf("unexpected NIST fallback result: %q, %v", content, err)
	}
}
