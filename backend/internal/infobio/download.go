package infobio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type DownloadService struct {
	ImagesURL   string
	NISTBaseURL string
	HTTPClient  *http.Client
	RequestHook RequestHook
	MaxRetries  int
	RetryDelay  time.Duration
	Search      *SearchService
}

func (service *DownloadService) JPGHandler() gin.HandlerFunc {
	return func(context *gin.Context) {
		alternatives := strings.Split(context.Query("alternative_nifs"), ",")
		image, err := service.DownloadJPG(context.Request.Context(), context.Param("nif"), alternatives)
		if err != nil {
			context.Error(err)
			context.Status(http.StatusBadGateway)
			return
		}
		context.Data(http.StatusOK, "image/jpeg", image.Content)
	}
}

func (service *DownloadService) PDFHandler() gin.HandlerFunc {
	return func(context *gin.Context) {
		content, err := service.DownloadPDF(context.Request.Context(), context.Param("nif"))
		if err != nil {
			context.Error(err)
			context.Status(http.StatusBadGateway)
			return
		}
		context.Data(http.StatusOK, "application/pdf", content)
	}
}

func (service *DownloadService) NISTHandler() gin.HandlerFunc {
	return func(context *gin.Context) {
		content, err := service.DownloadNIST(context.Request.Context(), context.Param("nif"), context.Query("path"))
		if err != nil {
			context.Error(err)
			context.Status(http.StatusBadGateway)
			return
		}
		context.Data(http.StatusOK, "application/octet-stream", content)
	}
}

type DownloadedImage struct {
	Content []byte
	NIF     string
}

type Availability struct {
	NIFExists      bool  `json:"nifExiste"`
	ImageAvailable bool  `json:"imagemDisponivel"`
	Data           Entry `json:"dados,omitempty"`
}

func NewDownloadService(imagesURL, nistBaseURL string, client *http.Client, hook RequestHook) *DownloadService {
	if client == nil {
		client = http.DefaultClient
	}
	return &DownloadService{
		ImagesURL: strings.TrimRight(imagesURL, "/"), NISTBaseURL: strings.TrimRight(nistBaseURL, "/"),
		HTTPClient: client, RequestHook: hook, MaxRetries: 3, RetryDelay: time.Second,
	}
}

func (service *DownloadService) DownloadJPG(ctx context.Context, nif string, alternatives []string) (DownloadedImage, error) {
	nifs := make([]string, 0, 10)
	nifs = append(nifs, nif)
	for _, alternative := range alternatives {
		if len(nifs) == 10 {
			break
		}
		if strings.TrimSpace(alternative) != "" {
			nifs = append(nifs, strings.TrimSpace(alternative))
		}
	}
	return service.downloadWithFallback(ctx, nifs, ".jpg", func(response *http.Response) error {
		contentType := strings.ToLower(response.Header.Get("Content-Type"))
		if !strings.Contains(contentType, "image") && !strings.Contains(contentType, "jpeg") && !strings.Contains(contentType, "jpg") {
			return fmt.Errorf("response is not an image: %s", contentType)
		}
		return nil
	})
}

func (service *DownloadService) DownloadPDF(ctx context.Context, nif string) ([]byte, error) {
	if strings.TrimSpace(nif) == "" {
		return nil, errors.New("nif is required")
	}
	body, status, err := service.get(ctx, service.ImagesURL, nif+".pdf")
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, fmt.Errorf("PDF not found for NIF %s", nif)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("InfoBio PDF request failed: HTTP %d", status)
	}
	return body, nil
}

func (service *DownloadService) DownloadNIST(ctx context.Context, nif, filePath string) ([]byte, error) {
	if strings.TrimSpace(nif) == "" {
		return nil, errors.New("nif is required")
	}
	nistURL, err := service.nistURL(nif, filePath)
	if err != nil {
		return nil, err
	}
	body, status, err := service.getURL(ctx, nistURL)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, fmt.Errorf("NIST file not found for NIF %s", nif)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("InfoBio NIST request failed: HTTP %d", status)
	}
	return body, nil
}

func (service *DownloadService) CheckAvailability(ctx context.Context, nif string) (Availability, error) {
	if service.Search == nil {
		return Availability{}, errors.New("infobio search service is not configured")
	}
	result, err := service.Search.Search(ctx, SearchParams{NIF: nif})
	if err != nil {
		return Availability{}, err
	}
	availability := Availability{}
	if len(result.Entries) == 0 {
		return availability, nil
	}
	availability.NIFExists = true
	availability.Data = result.Entries[0]
	_, err = service.DownloadJPG(ctx, nif, nil)
	availability.ImageAvailable = err == nil
	return availability, nil
}

func (service *DownloadService) downloadWithFallback(ctx context.Context, nifs []string, extension string, validate func(*http.Response) error) (DownloadedImage, error) {
	if len(nifs) == 0 || strings.TrimSpace(nifs[0]) == "" {
		return DownloadedImage{}, errors.New("nif is required")
	}
	maxRetries := service.MaxRetries
	if maxRetries < 1 {
		maxRetries = 1
	}
	var lastErr error
	for _, nif := range nifs {
		for attempt := 0; attempt < maxRetries; attempt++ {
			if attempt > 0 {
				time.Sleep(service.RetryDelay * time.Duration(attempt))
			}
			body, status, headers, err := service.getWithHeaders(ctx, service.ImagesURL, nif+extension)
			if err != nil {
				lastErr = err
				continue
			}
			if status == http.StatusOK {
				response := &http.Response{StatusCode: status, Header: headers}
				if err := validate(response); err != nil {
					lastErr = err
					continue
				}
				return DownloadedImage{Content: body, NIF: nif}, nil
			}
			lastErr = fmt.Errorf("InfoBio image request failed for NIF %s: HTTP %d", nif, status)
		}
	}
	if lastErr == nil {
		lastErr = errors.New("no InfoBio file available")
	}
	return DownloadedImage{}, fmt.Errorf("no image available for %d NIF(s): %w", len(nifs), lastErr)
}

func (service *DownloadService) get(ctx context.Context, baseURL, fileName string) ([]byte, int, error) {
	body, status, _, err := service.getWithHeaders(ctx, baseURL, fileName)
	return body, status, err
}

func (service *DownloadService) getURL(ctx context.Context, targetURL string) ([]byte, int, error) {
	body, status, _, err := service.getURLWithHeaders(ctx, targetURL)
	return body, status, err
}

func (service *DownloadService) getWithHeaders(ctx context.Context, baseURL, fileName string) ([]byte, int, http.Header, error) {
	return service.getURLWithHeaders(ctx, strings.TrimRight(baseURL, "/")+"/"+url.PathEscape(fileName))
}

func (service *DownloadService) getURLWithHeaders(ctx context.Context, targetURL string) ([]byte, int, http.Header, error) {
	if targetURL == "" {
		return nil, 0, nil, errors.New("InfoBio download URL is not configured")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, 0, nil, err
	}
	if service.RequestHook != nil {
		service.RequestHook(request)
	}
	response, err := service.HTTPClient.Do(request)
	if err != nil {
		return nil, 0, nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	return body, response.StatusCode, response.Header, err
}

func (service *DownloadService) nistURL(nif, filePath string) (string, error) {
	if filePath != "" {
		if strings.HasPrefix(filePath, "//") {
			return "http:" + filePath, nil
		}
		if strings.HasPrefix(filePath, "/") {
			if service.NISTBaseURL == "" {
				return "", errors.New("InfoBio NIST base URL is not configured")
			}
			base, err := url.Parse(service.NISTBaseURL)
			if err != nil {
				return "", err
			}
			base.Path = filePath
			base.RawPath = ""
			return base.String(), nil
		}
		return filePath, nil
	}
	if service.NISTBaseURL == "" {
		return "", errors.New("InfoBio NIST base URL is not configured")
	}
	return strings.TrimRight(service.NISTBaseURL, "/") + "/Nists/" + url.PathEscape(nif) + ".nist", nil
}
