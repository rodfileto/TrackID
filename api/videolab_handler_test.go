package api

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/rodfileto/trackid-vision/videoproc"

	"github.com/rodfileto/trackid/video"
)

func videoLabEngine(lab *video.Lab) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewVideoLab(lab)
	g := r.Group("/api")
	g.GET("videolab", h.InfoHandler())
	g.POST("videolab/jobs", h.UploadHandler())
	g.GET("videolab/jobs/:id", h.JobHandler())
	g.DELETE("videolab/jobs/:id", h.DeleteHandler())
	g.GET("videolab/jobs/:id/frame", h.FrameHandler())
	return r
}

func videoUpload(t *testing.T, filename string, data []byte, fields map[string]string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		_ = w.WriteField(k, v)
	}
	part, _ := w.CreateFormFile("file", filename)
	_, _ = part.Write(data)
	_ = w.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/videolab/jobs", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

func TestVideoLabDisabledWithoutTracker(t *testing.T) {
	r := videoLabEngine(video.NewLab(nil))
	info := call(t, r, http.MethodGet, "/api/videolab", nil)
	if info["enabled"] != false {
		t.Fatalf("info = %v", info)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, videoUpload(t, "clip.mp4", []byte("x"), nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("upload without tracker: %d %s", rec.Code, rec.Body)
	}
}

func TestVideoLabUploadPollFrameDelete(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not on PATH")
	}
	path := filepath.Join(t.TempDir(), "clip.mp4")
	if out, err := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc=size=160x120:rate=25:duration=2",
		"-pix_fmt", "yuv420p", path).CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v: %s", err, out)
	}
	data, _ := os.ReadFile(path)
	var interval float64
	lab := video.NewLab(func(_ context.Context, _ string, opts video.LabOptions, _ videoproc.ProgressFunc) (videoproc.Result, error) {
		interval = opts.IntervalSeconds
		f := videoproc.FaceDetection{FrameNumber: 2, TimestampSeconds: 1, Confidence: 0.9, Box: [4]float32{1, 1, 20, 20}, IsEmbedding: true}
		return videoproc.Result{Tracks: []videoproc.Track{{BestFace: f, AllFaces: []videoproc.FaceDetection{f}}}, TrackCount: 1}, nil
	})
	defer lab.Close()
	r := videoLabEngine(lab)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, videoUpload(t, "notes.txt", data, nil))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("not a video: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, videoUpload(t, "clip.mp4", data, map[string]string{"intervalSeconds": "99"}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("interval out of bounds: %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, videoUpload(t, "clip.mp4", data, map[string]string{"intervalSeconds": "0.5"}))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body)
	}
	var job video.LabJob
	_ = json.Unmarshal(rec.Body.Bytes(), &job)

	deadline := time.Now().Add(20 * time.Second)
	for job.Status != "DONE" && job.Status != "FAILED" && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
		rec = httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/videolab/jobs/"+job.ID, nil))
		_ = json.Unmarshal(rec.Body.Bytes(), &job)
	}
	if job.Status != "DONE" || len(job.Persons) != 1 || !job.Persons[0].Faces[0].Best || interval != 0.5 || job.FrameRate != 25 {
		t.Fatalf("job = %+v (interval %v)", job, interval)
	}

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/videolab/jobs/"+job.ID+"/frame?ms=1000", nil))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("frame: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/videolab/jobs/"+job.ID+"/frame?ms=abc", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad ms: %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/videolab/jobs/"+job.ID, nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/videolab/jobs/"+job.ID, nil))
	if rec.Code != http.StatusGone {
		t.Fatalf("after delete: %d", rec.Code)
	}
}

func TestExtractFrameRefusesBadFaces(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/cases/:caseId/videos/:fileId/frames", ExtractFrameHandler(nil, nil, nil))
	many := make([]map[string]any, video.MaxTracesPerFrame+1)
	for i := range many {
		many[i] = map[string]any{"box": []float32{0, 0, 10, 10}}
	}
	for name, body := range map[string]any{
		"inverted box": map[string]any{"timestampMs": 0, "faces": []map[string]any{{"box": []float32{10, 10, 0, 0}}}},
		"too many":     map[string]any{"timestampMs": 0, "faces": many},
	} {
		raw, _ := json.Marshal(body)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/cases/C1/videos/1/frames", bytes.NewReader(raw)))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", name, rec.Code, rec.Body)
		}
	}
}
