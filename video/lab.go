package video

import (
	"cmp"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/rodfileto/trackid-vision/videoproc"
	"github.com/rodfileto/trackid-vision/vision"
)

// The video lab: any video, not a case's, run through trackid-vision's videoproc, with every face
// it found grouped by person and the best one of each marked. Nothing goes to the database or to
// object storage: the video waits in a temporary file, for the page to ask for full frames, until
// it is removed or expires.

// LabTracker runs videoproc on a video file, reporting progress (videoproc.Process with a vision
// service in production; tests pass their own).
type LabTracker func(ctx context.Context, path string, opts LabOptions, onProgress videoproc.ProgressFunc) (videoproc.Result, error)

// LabOptions are the knobs the lab lets an examiner turn. Zero values take the defaults.
type LabOptions struct {
	// IntervalSeconds is the spacing of the sampled frames (default 1 s, from 0.1 to 10).
	IntervalSeconds float64 `json:"intervalSeconds"`
	// MinBlur leaves out faces less sharp than this (videoproc's blur score; default 30).
	MinBlur float64 `json:"minBlur"`
}

// Lab defaults and bounds.
const (
	LabDefaultInterval = 1.0
	LabMinInterval     = 0.1
	LabMaxInterval     = 10.0
	LabDefaultMinBlur  = 30.0
	LabMaxJobs         = 4
	LabJobTTL          = 2 * time.Hour
)

// Normalized returns the options with defaults filled in, or an error when out of bounds.
func (o LabOptions) Normalized() (LabOptions, error) {
	if o.IntervalSeconds == 0 {
		o.IntervalSeconds = LabDefaultInterval
	}
	if o.MinBlur == 0 {
		o.MinBlur = LabDefaultMinBlur
	}
	if math.IsNaN(o.IntervalSeconds) || o.IntervalSeconds < LabMinInterval || o.IntervalSeconds > LabMaxInterval {
		return o, fmt.Errorf("the sampling interval must be between %.1f and %.0f seconds", LabMinInterval, LabMaxInterval)
	}
	if math.IsNaN(o.MinBlur) || o.MinBlur < 0 || o.MinBlur > 10000 {
		return o, errors.New("the minimum sharpness must be between 0 and 10000")
	}
	return o, nil
}

// VisionLabTracker runs videoproc with vis, embedding every sampled frame, with the same
// clustering the case pipeline uses (VisionTracker).
func VisionLabTracker(vis *vision.Service) LabTracker {
	return func(ctx context.Context, path string, opts LabOptions, onProgress videoproc.ProgressFunc) (videoproc.Result, error) {
		return videoproc.Process(ctx, vis, path, videoproc.Options{IntervalSeconds: opts.IntervalSeconds, EmbedIntervalSeconds: opts.IntervalSeconds,
			MinBlur: opts.MinBlur, IoUThreshold: 0.3, ClusterEPS: 0.4, ClusterMinSamples: 2}, onProgress)
	}
}

// Lab errors.
var (
	ErrLabJobNotFound = errors.New("the video is no longer in the lab; send it again")
	ErrLabBusy        = errors.New("the lab already holds as many videos as it can while they are processed; remove one or wait")
	ErrLabNoTracker   = errors.New("face detection is not configured (VISION_DETECTOR_PATH/VISION_RECOGNIZER_PATH)")
)

// Face is one face videoproc found in one sampled frame, attributed to a person.
type Face struct {
	// TimestampMs is the sampled frame's time in the video.
	TimestampMs int64 `json:"timestampMs"`
	// Frame is the video frame number at that time (time × frame rate), when the rate is known.
	Frame *int64 `json:"frame,omitempty"`
	// Sample is the index of the sampled frame (0, 1, ... every IntervalSeconds).
	Sample     int        `json:"sample"`
	Box        [4]float32 `json:"box"`
	Confidence float32    `json:"confidence"`
	Blur       float64    `json:"blur"`
	// Embedded says the face was embedded and clustered itself, rather than attributed to a
	// person by its box overlapping an embedded face's.
	Embedded bool `json:"embedded"`
	// Best marks the person's best face (videoproc's choice: the highest detector confidence).
	Best bool   `json:"best"`
	Crop string `json:"crop"` // a data: URI, for display
}

// LabPerson is one track: every face believed to be the same person, in time order.
type LabPerson struct {
	Track   int    `json:"track"`
	Best    Face   `json:"best"`
	Faces   []Face `json:"faces"`
	FirstMs int64  `json:"firstMs"`
	LastMs  int64  `json:"lastMs"`
}

// LabJob is a video in the lab and its processing.
type LabJob struct {
	ID              string      `json:"id"`
	Filename        string      `json:"filename"`
	SHA256          string      `json:"sha256"`
	SizeBytes       int64       `json:"sizeBytes"`
	Options         LabOptions  `json:"options"`
	Status          string      `json:"status"` // PENDING | PROCESSING | DONE | FAILED
	Frames          int         `json:"frames"` // sampled frames processed so far
	ExpectedFrames  int         `json:"expectedFrames"`
	DurationSeconds float64     `json:"durationSeconds,omitempty"`
	FrameRate       float64     `json:"frameRate,omitempty"`
	Persons         []LabPerson `json:"persons"`
	Error           string      `json:"error,omitempty"`
	CreatedAt       time.Time   `json:"createdAt"`
	FinishedAt      *time.Time  `json:"finishedAt,omitempty"`
	Software        string      `json:"software"`
}

type labJob struct {
	LabJob
	dir, path string
	cancel    context.CancelFunc
	used      time.Time
}

// Lab holds the lab's videos. One video is processed at a time (the detector shares the
// machine, often a GPU, with everything else); the others wait.
type Lab struct {
	tracker LabTracker
	mu      sync.Mutex
	jobs    map[string]*labJob
	run     chan struct{} // a slot: one processing at a time
	now     func() time.Time
}

// NewLab returns an empty lab; a nil tracker makes Add fail with ErrLabNoTracker.
func NewLab(tracker LabTracker) *Lab {
	return &Lab{tracker: tracker, jobs: map[string]*labJob{}, run: make(chan struct{}, 1), now: time.Now}
}

// Enabled says whether the lab can process videos.
func (l *Lab) Enabled() bool { return l != nil && l.tracker != nil }

// Add copies a video from r (at most maxBytes) into a temporary file and starts processing it in
// the background. ErrNotVideo for a name not of a video format; the copy's error otherwise.
func (l *Lab) Add(filename string, r io.Reader, maxBytes int64, opts LabOptions) (LabJob, error) {
	if !l.Enabled() {
		return LabJob{}, ErrLabNoTracker
	}
	if contentType(filename) == "" {
		return LabJob{}, ErrNotVideo
	}
	opts, err := opts.Normalized()
	if err != nil {
		return LabJob{}, err
	}
	l.mu.Lock()
	l.expireLocked()
	if len(l.jobs) >= LabMaxJobs && !l.evictLocked() {
		l.mu.Unlock()
		return LabJob{}, ErrLabBusy
	}
	l.mu.Unlock()

	dir, err := os.MkdirTemp("", "trackid-videolab-")
	if err != nil {
		return LabJob{}, err
	}
	path := filepath.Join(dir, "video"+strings.ToLower(filepath.Ext(filename)))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		os.RemoveAll(dir)
		return LabJob{}, err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(r, maxBytes+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > maxBytes {
		err = fmt.Errorf("the video is larger than %d MB", maxBytes>>20)
	}
	if err == nil && n == 0 {
		err = ErrNotVideo
	}
	if err != nil {
		os.RemoveAll(dir)
		return LabJob{}, err
	}

	var idBytes [12]byte
	_, _ = rand.Read(idBytes[:])
	ctx, cancel := context.WithCancel(context.Background())
	now := l.now()
	j := &labJob{LabJob: LabJob{ID: hex.EncodeToString(idBytes[:]), Filename: filepath.Base(filename), SHA256: hex.EncodeToString(h.Sum(nil)),
		SizeBytes: n, Options: opts, Status: "PENDING", Persons: []LabPerson{}, CreatedAt: now, Software: Software()},
		dir: dir, path: path, cancel: cancel, used: now}
	l.mu.Lock()
	l.jobs[j.ID] = j
	out := j.snapshot()
	l.mu.Unlock()
	go l.process(ctx, j)
	return out, nil
}

// Software names what processes the lab's videos, for the page to show and a note to record.
func Software() string {
	return "trackid-vision videoproc (frame sampling with ffmpeg, face detection and embedding, DBSCAN clustering)"
}

func (j *labJob) snapshot() LabJob {
	out := j.LabJob
	out.Persons = slices.Clone(j.Persons)
	return out
}

// process runs the tracker, waiting for the lab's slot first.
func (l *Lab) process(ctx context.Context, j *labJob) {
	select {
	case l.run <- struct{}{}:
		defer func() { <-l.run }()
	case <-ctx.Done():
		return
	}
	finish := func(update func(*labJob)) {
		l.mu.Lock()
		defer l.mu.Unlock()
		update(j)
		t := l.now()
		j.FinishedAt = &t
	}
	duration, fps, err := probe(ctx, j.path)
	if err != nil {
		finish(func(j *labJob) { j.Status, j.Error = "FAILED", err.Error() })
		return
	}
	l.mu.Lock()
	j.Status, j.DurationSeconds, j.FrameRate = "PROCESSING", duration, fps
	j.ExpectedFrames = int(duration/j.Options.IntervalSeconds) + 1
	l.mu.Unlock()
	result, err := l.tracker(ctx, j.path, j.Options, func(frames, expected int) {
		l.mu.Lock()
		j.Frames, j.ExpectedFrames = frames, expected
		l.mu.Unlock()
	})
	if err != nil {
		if ctx.Err() != nil {
			return // removed while processing
		}
		finish(func(j *labJob) { j.Status, j.Error = "FAILED", err.Error() })
		return
	}
	persons := LabPersons(result, fps)
	finish(func(j *labJob) { j.Status, j.Persons, j.Frames = "DONE", persons, j.ExpectedFrames })
}

// LabPersons turns videoproc's tracks into the lab's persons: faces in time order, the best one
// marked, frame numbers from fps (0 leaves them out). Persons come in videoproc's track order.
func LabPersons(result videoproc.Result, fps float64) []LabPerson {
	persons := make([]LabPerson, 0, len(result.Tracks))
	for _, t := range result.Tracks {
		best, faces := trackFaces(t, fps)
		p := LabPerson{Track: t.TrackID, Best: best, Faces: faces}
		if len(faces) > 0 {
			p.FirstMs, p.LastMs = faces[0].TimestampMs, faces[len(faces)-1].TimestampMs
		}
		persons = append(persons, p)
	}
	return persons
}

// faceOf is one videoproc detection as a Face, its frame number from fps (0 leaves it out).
func faceOf(d videoproc.FaceDetection, fps float64, best bool) Face {
	f := Face{TimestampMs: ms(d.TimestampSeconds), Sample: d.FrameNumber, Box: d.Box, Confidence: d.Confidence,
		Blur: d.BlurScore, Embedded: d.IsEmbedding, Best: best, Crop: d.FaceCrop}
	if fps > 0 {
		n := int64(math.Round(d.TimestampSeconds * fps))
		f.Frame = &n
	}
	return f
}

// trackFaces returns a track's best face and all its faces in time order, the best one marked.
func trackFaces(t videoproc.Track, fps float64) (Face, []Face) {
	faces := make([]Face, 0, len(t.AllFaces))
	bestMarked := false
	for _, d := range t.AllFaces {
		isBest := !bestMarked && d.FrameNumber == t.BestFace.FrameNumber && d.Box == t.BestFace.Box
		bestMarked = bestMarked || isBest
		faces = append(faces, faceOf(d, fps, isBest))
	}
	slices.SortStableFunc(faces, func(a, b Face) int {
		return cmp.Or(cmp.Compare(a.TimestampMs, b.TimestampMs), cmp.Compare(a.Box[0], b.Box[0]))
	})
	return faceOf(t.BestFace, fps, true), faces
}

// Get returns a job as it stands.
func (l *Lab) Get(id string) (LabJob, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.expireLocked()
	j, ok := l.jobs[id]
	if !ok {
		return LabJob{}, ErrLabJobNotFound
	}
	j.used = l.now()
	return j.snapshot(), nil
}

// Remove stops a job if it is running and deletes its video.
func (l *Lab) Remove(id string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	j, ok := l.jobs[id]
	if !ok {
		return ErrLabJobNotFound
	}
	l.dropLocked(j)
	return nil
}

// Frame extracts the full frame at timestampMs of a job's video, as a PNG.
func (l *Lab) Frame(ctx context.Context, id string, timestampMs int64) ([]byte, error) {
	l.mu.Lock()
	l.expireLocked()
	j, ok := l.jobs[id]
	var path string
	var duration float64
	if ok {
		j.used = l.now()
		path, duration = j.path, j.DurationSeconds
	}
	l.mu.Unlock()
	if !ok {
		return nil, ErrLabJobNotFound
	}
	if timestampMs < 0 || (duration > 0 && float64(timestampMs) > duration*1000) {
		return nil, ErrOutOfRange
	}
	return frameAt(ctx, path, timestampMs)
}

// Close stops every job and deletes the videos.
func (l *Lab) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, j := range l.jobs {
		l.dropLocked(j)
	}
}

func (l *Lab) dropLocked(j *labJob) {
	j.cancel()
	os.RemoveAll(j.dir)
	delete(l.jobs, j.ID)
}

// expireLocked drops jobs unused for LabJobTTL.
func (l *Lab) expireLocked() {
	now := l.now()
	for _, j := range l.jobs {
		if now.Sub(j.used) > LabJobTTL {
			l.dropLocked(j)
		}
	}
}

// evictLocked drops the least recently used finished job, to make room; false when every job is
// still pending or processing.
func (l *Lab) evictLocked() bool {
	var oldest *labJob
	for _, j := range l.jobs {
		if j.Status != "DONE" && j.Status != "FAILED" {
			continue
		}
		if oldest == nil || j.used.Before(oldest.used) {
			oldest = j
		}
	}
	if oldest == nil {
		return false
	}
	l.dropLocked(oldest)
	return true
}
