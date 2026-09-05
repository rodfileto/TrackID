// Package vision runs face detection (SCRFD) and face embedding (ArcFace-style
// recognition, here AuraFace-v1) directly in the Go process via onnxruntime,
// replacing the Python ML sidecar. It re-implements the pre/post-processing
// insightface's Python FaceAnalysis does (blob construction, anchor decoding,
// NMS, 5-point similarity alignment) so results match that reference.
package vision

import (
	"fmt"
	"log"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
)

var initOnce sync.Once
var initErr error

// defaultSharedLibraryPath points at the GPU-capable onnxruntime build
// vendored under backend-go/onnxruntime-gpu/ (relative to the working
// directory the server is started from, matching the models/ path
// convention). This build works fine for CPU-only inference too -- it's a
// superset of the apt-packaged CPU-only libonnxruntime1.23, just also able
// to load the CUDA execution provider when asked.
const defaultSharedLibraryPath = "onnxruntime-gpu/libonnxruntime.so"

func ensureInitialized(sharedLibPath string) error {
	initOnce.Do(func() {
		if sharedLibPath == "" {
			sharedLibPath = defaultSharedLibraryPath
		}
		ort.SetSharedLibraryPath(sharedLibPath)
		initErr = ort.InitializeEnvironment()
	})
	return initErr
}

// Service holds the loaded detector and recognizer sessions.
type Service struct {
	detector   *ort.DynamicAdvancedSession
	recognizer *ort.DynamicAdvancedSession
}

// Config points at the two AuraFace-v1 ONNX files and, optionally, a
// non-default onnxruntime shared library path.
type Config struct {
	DetectorPath      string
	RecognizerPath    string
	SharedLibraryPath string

	// UseGPU requests the CUDA execution provider. If the GPU or CUDA
	// libraries aren't available, NewService logs a warning and falls back
	// to CPU rather than failing outright -- a missing GPU shouldn't take
	// the server down.
	UseGPU   bool
	DeviceID int
}

func NewService(cfg Config) (*Service, error) {
	if err := ensureInitialized(cfg.SharedLibraryPath); err != nil {
		return nil, fmt.Errorf("initialize onnxruntime: %w", err)
	}

	sessionOpts, cleanup := buildSessionOptions(cfg)
	defer cleanup()

	detector, err := ort.NewDynamicAdvancedSession(
		cfg.DetectorPath,
		[]string{detectorInputName},
		detectorOutputNames,
		sessionOpts,
	)
	if err != nil {
		return nil, fmt.Errorf("load detector model: %w", err)
	}

	recognizer, err := ort.NewDynamicAdvancedSession(
		cfg.RecognizerPath,
		[]string{recognizerInputName},
		[]string{recognizerOutputName},
		sessionOpts,
	)
	if err != nil {
		detector.Destroy()
		return nil, fmt.Errorf("load recognizer model: %w", err)
	}

	return &Service{detector: detector, recognizer: recognizer}, nil
}

// buildSessionOptions returns CUDA-enabled session options when requested
// and available, or nil (onnxruntime's CPU default) otherwise. The returned
// cleanup func must run once both sessions have been created from it.
func buildSessionOptions(cfg Config) (opts *ort.SessionOptions, cleanup func()) {
	noop := func() {}
	if !cfg.UseGPU {
		return nil, noop
	}

	opts, err := ort.NewSessionOptions()
	if err != nil {
		log.Printf("vision: could not create session options, falling back to CPU: %v", err)
		return nil, noop
	}

	cudaOpts, err := ort.NewCUDAProviderOptions()
	if err != nil {
		log.Printf("vision: CUDA execution provider unavailable, falling back to CPU: %v", err)
		opts.Destroy()
		return nil, noop
	}
	defer cudaOpts.Destroy()

	if err := cudaOpts.Update(map[string]string{"device_id": fmt.Sprintf("%d", cfg.DeviceID)}); err != nil {
		log.Printf("vision: could not configure CUDA provider, falling back to CPU: %v", err)
		opts.Destroy()
		return nil, noop
	}

	if err := opts.AppendExecutionProviderCUDA(cudaOpts); err != nil {
		log.Printf("vision: could not enable CUDA execution provider, falling back to CPU: %v", err)
		opts.Destroy()
		return nil, noop
	}

	log.Printf("vision: CUDA execution provider enabled (device %d)", cfg.DeviceID)
	return opts, func() { opts.Destroy() }
}

func (s *Service) Close() {
	if s.detector != nil {
		s.detector.Destroy()
	}
	if s.recognizer != nil {
		s.recognizer.Destroy()
	}
}
