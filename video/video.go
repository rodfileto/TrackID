// Package video takes video sent in as material (docs/facial-reports-plan.md, decision 14). The
// video is kept as received, a case file of category 'video' with its SHA-256. The worker samples
// it with trackid-vision's videoproc, which detects and embeds the faces and groups them into one
// track per person; the examiner then picks frames, and each pick becomes an evidence -- the full
// frame, extracted by ffmpeg at its time -- that records the video, the time and the frame number,
// so a report can say exactly where a face came from.
//
// Needs ffmpeg and ffprobe on the PATH of the processes that run it (the worker, and the API for
// extracting frames).
package video

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/rodfileto/trackid-vision/videoproc"
	"github.com/rodfileto/trackid-vision/vision"
	"github.com/sqlc-dev/pqtype"

	"github.com/rodfileto/trackid/cases"
	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/embedding"
	"github.com/rodfileto/trackid/storage"
)

// TaskTypeProcess is the worker task that processes one video (HandleProcess).
const TaskTypeProcess = "video:process"

// MaxBytes bounds an uploaded video.
const MaxBytes = 1 << 30

// Errors returned to callers.
var (
	ErrCaseNotFound = errors.New("case not found")
	ErrNotFound     = errors.New("video not found")
	ErrNotVideo     = errors.New("unsupported video; allowed: mp4, mov, avi, mkv, webm")
	ErrNotProcessed = errors.New("the video has not been processed yet")
	ErrOutOfRange   = errors.New("the time is outside the video")
	ErrNoStore      = errors.New("object storage is not configured")
	errNoFrame      = errors.New("video: ffmpeg returned no frame")
	videoExtensions = map[string]string{".mp4": "video/mp4", ".m4v": "video/mp4", ".mov": "video/quicktime", ".avi": "video/x-msvideo", ".mkv": "video/x-matroska", ".webm": "video/webm"}
)

// Track is one person videoproc followed through the video: the best face found (its time, box,
// sharpness and a small crop to show) and the span of its detections.
type Track struct {
	Track       int        `json:"track"`
	TimestampMs int64      `json:"timestampMs"`
	Box         [4]float32 `json:"box"`
	Blur        float64    `json:"blur"`
	Confidence  float32    `json:"confidence"`
	Crop        string     `json:"crop"` // a data: URI
	Detections  int        `json:"detections"`
	FirstMs     int64      `json:"firstMs"`
	LastMs      int64      `json:"lastMs"`
}

// Video is one received video and its processing.
type Video struct {
	FileID          int64      `json:"fileId"`
	Filename        string     `json:"filename"`
	SHA256          string     `json:"sha256"`
	ContentType     string     `json:"contentType,omitempty"`
	SizeBytes       int64      `json:"sizeBytes"`
	RequestID       int64      `json:"requestId,omitempty"`
	Status          string     `json:"status"` // PENDING | PROCESSING | DONE | FAILED
	DurationSeconds float64    `json:"durationSeconds,omitempty"`
	FrameRate       float64    `json:"frameRate,omitempty"`
	Tracks          []Track    `json:"tracks"`
	Error           string     `json:"error,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
	ProcessedAt     *time.Time `json:"processedAt,omitempty"`
}

// contentType is a video's type from its name, or "" when it is not one this package takes.
func contentType(filename string) string {
	return videoExtensions[strings.ToLower(filepath.Ext(filename))]
}

// IsVideo reports whether filename is of a video format Add takes.
func IsVideo(filename string) bool { return contentType(filename) != "" }

// Add stores a video sent in for a case and queues its processing (a nil queue leaves it pending).
// The same bytes added again are the same video.
func Add(ctx context.Context, sqlDB *sql.DB, store *storage.Client, queue embedding.Enqueuer, caseID, filename string, data []byte) (Video, error) {
	if store == nil {
		return Video{}, ErrNoStore
	}
	ct := contentType(filename)
	if ct == "" || len(data) == 0 {
		return Video{}, ErrNotVideo
	}
	q := db.New(sqlDB)
	c, err := q.GetCaseByCaseID(ctx, caseID)
	if errors.Is(err, sql.ErrNoRows) {
		return Video{}, ErrCaseNotFound
	}
	if err != nil {
		return Video{}, err
	}
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	ref, err := store.Upload(ctx, fmt.Sprintf("videos/%s/%s", caseID, hash), data, ct)
	if err != nil {
		return Video{}, fmt.Errorf("video: upload: %w", err)
	}
	row, err := q.UpsertCaseFile(ctx, db.UpsertCaseFileParams{BiometricCaseID: c.ID, Category: "video",
		MediaType: sql.NullString{String: "video", Valid: true}, HashID: sql.NullString{String: hash, Valid: true},
		Filename: sql.NullString{String: filename, Valid: true}, StorageRef: sql.NullString{String: ref, Valid: true},
		ContentType: sql.NullString{String: ct, Valid: true}, SizeBytes: sql.NullInt64{Int64: int64(len(data)), Valid: true}})
	if err != nil {
		return Video{}, err
	}
	if err := q.InsertCaseVideo(ctx, row.ID); err != nil {
		return Video{}, err
	}
	if queue != nil {
		payload, _ := json.Marshal(map[string]int64{"fileId": row.ID})
		if _, err := queue.Enqueue(asynq.NewTask(TaskTypeProcess, payload), asynq.Timeout(30*time.Minute)); err != nil {
			return Video{}, fmt.Errorf("video: enqueue: %w", err)
		}
	}
	return Get(ctx, sqlDB, caseID, row.ID)
}

// List returns a case's videos with their tracks.
func List(ctx context.Context, sqlDB *sql.DB, caseID string) ([]Video, error) {
	q := db.New(sqlDB)
	if _, err := q.GetCaseByCaseID(ctx, caseID); errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCaseNotFound
	} else if err != nil {
		return nil, err
	}
	rows, err := q.ListCaseVideos(ctx, caseID)
	if err != nil {
		return nil, err
	}
	out := make([]Video, 0, len(rows))
	for _, r := range rows {
		v := Video{FileID: r.CaseFileID, Filename: r.Filename.String, SHA256: r.HashID.String, ContentType: r.ContentType.String,
			SizeBytes: r.SizeBytes.Int64, RequestID: r.CaseRequestID.Int64, Status: r.Status, DurationSeconds: r.DurationSeconds.Float64,
			FrameRate: r.FrameRate.Float64, Error: r.Error.String, CreatedAt: r.CreatedAt, Tracks: []Track{}}
		if r.ProcessedAt.Valid {
			v.ProcessedAt = &r.ProcessedAt.Time
		}
		if r.Tracks.Valid {
			if err := json.Unmarshal(r.Tracks.RawMessage, &v.Tracks); err != nil {
				return nil, err
			}
		}
		out = append(out, v)
	}
	return out, nil
}

// Get returns one video of a case.
func Get(ctx context.Context, sqlDB *sql.DB, caseID string, fileID int64) (Video, error) {
	list, err := List(ctx, sqlDB, caseID)
	if err != nil {
		return Video{}, err
	}
	for _, v := range list {
		if v.FileID == fileID {
			return v, nil
		}
	}
	return Video{}, ErrNotFound
}

// Tracker finds the tracks of a video file (videoproc.Process in production).
type Tracker func(ctx context.Context, path string) (videoproc.Result, error)

// VisionTracker runs videoproc with vis: one sample a second, every sample embedded, faces too
// blurred to compare left out.
func VisionTracker(vis *vision.Service) Tracker {
	return func(ctx context.Context, path string) (videoproc.Result, error) {
		return videoproc.Process(ctx, vis, path, videoproc.Options{IntervalSeconds: 1, EmbedIntervalSeconds: 1, MinBlur: 30,
			IoUThreshold: 0.3, ClusterEPS: 0.4, ClusterMinSamples: 2}, nil)
	}
}

// download writes a stored video to a temporary file; the caller removes its directory.
func download(ctx context.Context, store *storage.Client, ref, filename string) (string, string, error) {
	data, err := store.Download(ctx, ref)
	if err != nil {
		return "", "", fmt.Errorf("video: download: %w", err)
	}
	dir, err := os.MkdirTemp("", "trackid-video-")
	if err != nil {
		return "", "", err
	}
	path := filepath.Join(dir, "video"+strings.ToLower(filepath.Ext(filename)))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		os.RemoveAll(dir)
		return "", "", err
	}
	return dir, path, nil
}

// Process runs the tracker on a stored video and records its tracks, or the failure.
func Process(ctx context.Context, sqlDB *sql.DB, store *storage.Client, track Tracker, fileID int64) error {
	q := db.New(sqlDB)
	v, err := q.GetCaseVideo(ctx, fileID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if err := q.StartCaseVideo(ctx, fileID); err != nil {
		return err
	}
	fail := func(err error) error {
		_ = q.FailCaseVideo(ctx, db.FailCaseVideoParams{CaseFileID: fileID, Error: sql.NullString{String: err.Error(), Valid: true}})
		return err
	}
	dir, path, err := download(ctx, store, v.StorageRef.String, v.Filename.String)
	if err != nil {
		return fail(err)
	}
	defer os.RemoveAll(dir)
	duration, fps, err := probe(ctx, path)
	if err != nil {
		return fail(err)
	}
	result, err := track(ctx, path)
	if err != nil {
		return fail(err)
	}
	tracks := make([]Track, 0, len(result.Tracks))
	for _, t := range result.Tracks {
		out := Track{Track: t.TrackID, TimestampMs: ms(t.BestFace.TimestampSeconds), Box: t.BestFace.Box, Blur: t.BestFace.BlurScore,
			Confidence: t.BestFace.Confidence, Crop: t.BestFace.FaceCrop, Detections: len(t.AllFaces)}
		for i, f := range t.AllFaces {
			if i == 0 || ms(f.TimestampSeconds) < out.FirstMs {
				out.FirstMs = ms(f.TimestampSeconds)
			}
			if ms(f.TimestampSeconds) > out.LastMs {
				out.LastMs = ms(f.TimestampSeconds)
			}
		}
		tracks = append(tracks, out)
	}
	raw, err := json.Marshal(tracks)
	if err != nil {
		return fail(err)
	}
	return q.FinishCaseVideo(ctx, db.FinishCaseVideoParams{CaseFileID: fileID, DurationSeconds: sql.NullFloat64{Float64: duration, Valid: true},
		FrameRate: sql.NullFloat64{Float64: fps, Valid: fps > 0}, Tracks: pqtype.NullRawMessage{RawMessage: raw, Valid: true}})
}

func ms(seconds float64) int64 { return int64(math.Round(seconds * 1000)) }

// HandleProcess is the worker's handler for TaskTypeProcess.
func HandleProcess(sqlDB *sql.DB, store *storage.Client, track Tracker) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, task *asynq.Task) error {
		var p struct {
			FileID int64 `json:"fileId"`
		}
		if err := json.Unmarshal(task.Payload(), &p); err != nil {
			return fmt.Errorf("video: payload: %w: %w", err, asynq.SkipRetry)
		}
		if err := Process(ctx, sqlDB, store, track, p.FileID); err != nil {
			return fmt.Errorf("%w: %w", err, asynq.SkipRetry) // the failure is recorded on the video; retrying will not change it
		}
		return nil
	}
}

// probe reads a video's duration and frame rate with ffprobe.
func probe(ctx context.Context, path string) (duration, fps float64, err error) {
	cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=r_frame_rate:format=duration", "-of", "default=noprint_wrappers=1", path)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return 0, 0, fmt.Errorf("ffprobe: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	for _, line := range strings.Split(string(out), "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch k {
		case "duration":
			duration, _ = strconv.ParseFloat(v, 64)
		case "r_frame_rate":
			if n, d, ok := strings.Cut(v, "/"); ok {
				num, _ := strconv.ParseFloat(n, 64)
				den, _ := strconv.ParseFloat(d, 64)
				if den > 0 {
					fps = num / den
				}
			}
		}
	}
	if duration <= 0 {
		return 0, 0, ErrNotVideo
	}
	return duration, fps, nil
}

// frameAt extracts the frame shown at timestampMs as a PNG.
func frameAt(ctx context.Context, path string, timestampMs int64) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-ss", fmt.Sprintf("%.3f", float64(timestampMs)/1000),
		"-i", path, "-frames:v", "1", "-f", "image2", "-c:v", "png", "pipe:1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	if len(out) == 0 {
		return nil, errNoFrame
	}
	return out, nil
}

// Clock formats a time in a video as HH:MM:SS.mmm.
func Clock(timestampMs int64) string {
	d := time.Duration(timestampMs) * time.Millisecond
	return fmt.Sprintf("%02d:%02d:%02d.%03d", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60, timestampMs%1000)
}

// Extracted is a frame taken from a video as an evidence, with the face marked on it.
type Extracted struct {
	Evidence cases.File    `json:"evidence"`
	Traces   []cases.Trace `json:"traces"`
}

// Extract takes the frame of a processed video at timestampMs as a new evidence of the case,
// recording the video, the time and the frame number; box, when given, marks the face on it as a
// trace (from a track, or drawn by the examiner later otherwise). The frame is the full image, not
// the track's small crop.
func Extract(ctx context.Context, sqlDB *sql.DB, store *storage.Client, queue embedding.Enqueuer, caseID string, fileID, timestampMs int64,
	box *[4]float32) (Extracted, error) {
	if store == nil {
		return Extracted{}, ErrNoStore
	}
	q := db.New(sqlDB)
	v, err := q.GetCaseVideo(ctx, fileID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && v.CaseID != caseID) {
		return Extracted{}, ErrNotFound
	}
	if err != nil {
		return Extracted{}, err
	}
	if v.Status != "DONE" {
		return Extracted{}, ErrNotProcessed
	}
	if timestampMs < 0 || float64(timestampMs) > v.DurationSeconds.Float64*1000 {
		return Extracted{}, ErrOutOfRange
	}
	dir, path, err := download(ctx, store, v.StorageRef.String, v.Filename.String)
	if err != nil {
		return Extracted{}, err
	}
	defer os.RemoveAll(dir)
	frame, err := frameAt(ctx, path, timestampMs)
	if err != nil {
		return Extracted{}, err
	}
	name := fmt.Sprintf("%s @ %s.png", strings.TrimSuffix(v.Filename.String, filepath.Ext(v.Filename.String)), strings.ReplaceAll(Clock(timestampMs), ":", "-"))
	file, err := cases.AddEvidence(ctx, sqlDB, store, caseID, cases.EvidenceFileInput{Filename: name, Data: frame})
	if err != nil {
		return Extracted{}, err
	}
	out := Extracted{Evidence: file, Traces: []cases.Trace{}}
	if box != nil {
		if out.Traces, err = cases.CreateTraces(ctx, sqlDB, queue, caseID, file.ID, []cases.TraceDetection{{
			BoxX1: float64(box[0]), BoxY1: float64(box[1]), BoxX2: float64(box[2]), BoxY2: float64(box[3]), Score: 1}}); err != nil {
			return Extracted{}, err
		}
	} else if _, err := q.CreateCaseEvidenceForFile(ctx, db.CreateCaseEvidenceForFileParams{BiometricCaseID: v.BiometricCaseID,
		CaseFileID: sql.NullInt64{Int64: file.ID, Valid: true}}); err != nil {
		return Extracted{}, err // no face marked yet: the evidence row still records where the frame came from
	}
	frameNumber := sql.NullInt64{}
	if v.FrameRate.Valid && v.FrameRate.Float64 > 0 {
		frameNumber = sql.NullInt64{Int64: int64(math.Round(float64(timestampMs) / 1000 * v.FrameRate.Float64)), Valid: true}
	}
	n, err := q.SetEvidenceVideoSource(ctx, db.SetEvidenceVideoSourceParams{CaseFileID: sql.NullInt64{Int64: file.ID, Valid: true},
		SourceVideoFileID: sql.NullInt64{Int64: fileID, Valid: true}, SourceTimestampMs: sql.NullInt64{Int64: timestampMs, Valid: true},
		SourceFrame: frameNumber})
	if err != nil {
		return Extracted{}, err
	}
	if n == 0 {
		return Extracted{}, fmt.Errorf("video: the frame's evidence row was not created")
	}
	return out, nil
}
