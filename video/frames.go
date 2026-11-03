package video

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png" // frameAt's output
	"math"
	"os"
	"sync"
	"time"

	"github.com/hibiken/asynq"
	"github.com/rodfileto/trackid-vision/vision"

	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/embedding"
	"github.com/rodfileto/trackid/storage"
)

// Working on a case's video from the case page: play it, look at the faces in one frame, and take
// that frame as an evidence with the faces chosen marked on it as traces.

// FrameFace is a face found in a frame: its box in the frame's pixels and the detector's score,
// and the person (track) it belongs to when it matches one of that person's faces.
type FrameFace struct {
	Box   [4]float32 `json:"box"`
	Score float32    `json:"score"`
	Track *int       `json:"track,omitempty"`
}

// Frame is one frame of a case's video with the faces in it.
type Frame struct {
	TimestampMs int64       `json:"timestampMs"`
	Frame       *int64      `json:"frame,omitempty"`
	Width       int         `json:"width"`
	Height      int         `json:"height"`
	Image       string      `json:"image"` // a JPEG data: URI, for display; the evidence is taken as PNG
	Faces       []FrameFace `json:"faces"`
	// Detected says the faces were found in this very frame; false means vision is not configured
	// and they are the tracks' faces at the nearest sampled time.
	Detected bool `json:"detected"`
}

// --- A local copy of the videos the API works on -------------------------------------------------

// The API downloads a case's video from object storage to work on it; the copy is kept for a while
// so playing it, looking at frames and taking several of them does not download it each time.
const (
	cacheMaxFiles = 3
	cacheTTL      = 30 * time.Minute
)

type cachedVideo struct {
	dir, path string
	used      time.Time
	refs      int
	ready     chan struct{}
	err       error
}

type videoCache struct {
	mu    sync.Mutex
	files map[int64]*cachedVideo
}

var localVideos = &videoCache{files: map[int64]*cachedVideo{}}

// get returns the local path of a stored video, downloading it if needed; release must be called
// when done with the path.
func (c *videoCache) get(ctx context.Context, store *storage.Client, fileID int64, ref, filename string) (string, func(), error) {
	c.mu.Lock()
	c.sweepLocked()
	v, ok := c.files[fileID]
	if !ok {
		v = &cachedVideo{ready: make(chan struct{})}
		c.files[fileID] = v
		go func() {
			// Downloaded without the request's context: a cancelled request should not leave a
			// broken copy for the next one.
			dctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			dir, path, err := download(dctx, store, ref, filename)
			c.mu.Lock()
			v.dir, v.path, v.err = dir, path, err
			if err != nil {
				delete(c.files, fileID)
			}
			c.mu.Unlock()
			close(v.ready)
		}()
	}
	v.refs++
	v.used = time.Now()
	c.mu.Unlock()
	release := func() {
		c.mu.Lock()
		v.refs--
		v.used = time.Now()
		c.mu.Unlock()
	}
	select {
	case <-v.ready:
	case <-ctx.Done():
		release()
		return "", nil, ctx.Err()
	}
	if v.err != nil {
		release()
		return "", nil, v.err
	}
	return v.path, release, nil
}

// sweepLocked removes copies unused for cacheTTL, and the least recently used ones over
// cacheMaxFiles, never one in use.
func (c *videoCache) sweepLocked() {
	now := time.Now()
	for id, v := range c.files {
		if v.refs == 0 && v.path != "" && now.Sub(v.used) > cacheTTL {
			os.RemoveAll(v.dir)
			delete(c.files, id)
		}
	}
	for len(c.files) >= cacheMaxFiles {
		var oldest int64 = -1
		for id, v := range c.files {
			if v.refs == 0 && v.path != "" && (oldest < 0 || v.used.Before(c.files[oldest].used)) {
				oldest = id
			}
		}
		if oldest < 0 {
			return
		}
		os.RemoveAll(c.files[oldest].dir)
		delete(c.files, oldest)
	}
}

// --- Opening a case's video ---------------------------------------------------------------------

// Opened is a case's video file on local disk, for serving it; Close releases it.
type Opened struct {
	Path        string
	Filename    string
	ContentType string
	release     func()
}

// Close releases the local copy.
func (o Opened) Close() {
	if o.release != nil {
		o.release()
	}
}

func caseVideo(ctx context.Context, sqlDB *sql.DB, caseID string, fileID int64) (db.GetCaseVideoRow, error) {
	v, err := db.New(sqlDB).GetCaseVideo(ctx, fileID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && v.CaseID != caseID) {
		return db.GetCaseVideoRow{}, ErrNotFound
	}
	return v, err
}

// Open returns a local copy of a case's video, to play it.
func Open(ctx context.Context, sqlDB *sql.DB, store *storage.Client, caseID string, fileID int64) (Opened, error) {
	if store == nil {
		return Opened{}, ErrNoStore
	}
	v, err := caseVideo(ctx, sqlDB, caseID, fileID)
	if err != nil {
		return Opened{}, err
	}
	path, release, err := localVideos.get(ctx, store, fileID, v.StorageRef.String, v.Filename.String)
	if err != nil {
		return Opened{}, err
	}
	ct := v.ContentType.String
	if ct == "" {
		ct = contentType(v.Filename.String)
	}
	return Opened{Path: path, Filename: v.Filename.String, ContentType: ct, release: release}, nil
}

// --- Faces in one frame -------------------------------------------------------------------------

// trackIoU is the overlap a detected face needs with a track's face to be taken as that person.
const trackIoU = 0.3

// FrameFaces extracts the frame at timestampMs of a processed case video and finds the faces in
// it with detector, matching each to a person when it overlaps one of that person's faces at the
// nearest sampled time. A nil detector returns the tracks' faces at the nearest sampled time.
func FrameFaces(ctx context.Context, sqlDB *sql.DB, store *storage.Client, detector func([]byte) ([]vision.Detection, error),
	caseID string, fileID, timestampMs int64) (Frame, error) {
	if store == nil {
		return Frame{}, ErrNoStore
	}
	v, err := caseVideo(ctx, sqlDB, caseID, fileID)
	if err != nil {
		return Frame{}, err
	}
	if v.Status != "DONE" {
		return Frame{}, ErrNotProcessed
	}
	if timestampMs < 0 || float64(timestampMs) > v.DurationSeconds.Float64*1000 {
		return Frame{}, ErrOutOfRange
	}
	var tracks []Track
	if v.Tracks.Valid {
		if err := json.Unmarshal(v.Tracks.RawMessage, &tracks); err != nil {
			return Frame{}, err
		}
	}
	path, release, err := localVideos.get(ctx, store, fileID, v.StorageRef.String, v.Filename.String)
	if err != nil {
		return Frame{}, err
	}
	defer release()
	png, err := frameAt(ctx, path, timestampMs)
	if err != nil {
		return Frame{}, err
	}
	img, _, err := image.Decode(bytes.NewReader(png))
	if err != nil {
		return Frame{}, fmt.Errorf("video: decode frame: %w", err)
	}
	var shown bytes.Buffer
	if err := jpeg.Encode(&shown, img, &jpeg.Options{Quality: 92}); err != nil {
		return Frame{}, err
	}
	out := Frame{TimestampMs: timestampMs, Width: img.Bounds().Dx(), Height: img.Bounds().Dy(),
		Image: "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(shown.Bytes()), Faces: []FrameFace{}}
	if v.FrameRate.Valid && v.FrameRate.Float64 > 0 {
		n := int64(math.Round(float64(timestampMs) / 1000 * v.FrameRate.Float64))
		out.Frame = &n
	}
	near := nearestTrackFaces(tracks, timestampMs)
	if detector == nil {
		for _, tf := range near {
			track := tf.track
			out.Faces = append(out.Faces, FrameFace{Box: tf.face.Box, Score: tf.face.Confidence, Track: &track})
		}
		return out, nil
	}
	dets, err := detector(png)
	if err != nil {
		return Frame{}, fmt.Errorf("video: detect faces: %w", err)
	}
	out.Detected = true
	for _, d := range dets {
		f := FrameFace{Box: d.Box, Score: d.Score}
		best := float32(trackIoU)
		for _, tf := range near {
			if iou := vision.IoU(d.Box, tf.face.Box); iou > best {
				best = iou
				track := tf.track
				f.Track = &track
			}
		}
		out.Faces = append(out.Faces, f)
	}
	return out, nil
}

type trackFace struct {
	track int
	face  Face
}

// nearestTrackFaces returns the tracks' faces at the sampled time nearest timestampMs (within
// half the sampling interval, which the case pipeline sets at one second).
func nearestTrackFaces(tracks []Track, timestampMs int64) []trackFace {
	var out []trackFace
	nearest := int64(math.MaxInt64)
	for _, t := range tracks {
		faces := t.Faces
		if len(faces) == 0 { // processed before every face was recorded: the best one only
			faces = []Face{{TimestampMs: t.TimestampMs, Box: t.Box, Confidence: t.Confidence}}
		}
		for _, f := range faces {
			d := f.TimestampMs - timestampMs
			if d < 0 {
				d = -d
			}
			if d > 500 {
				continue
			}
			if d < nearest {
				nearest, out = d, nil
			}
			if d == nearest {
				out = append(out, trackFace{track: t.Track, face: f})
			}
		}
	}
	return out
}

// --- Processing again ---------------------------------------------------------------------------

// Reprocess queues a case's video to be processed again (after a failure, or to record every
// face of each person for a video processed before that was kept).
func Reprocess(ctx context.Context, sqlDB *sql.DB, queue embedding.Enqueuer, caseID string, fileID int64) error {
	if queue == nil {
		return ErrNoQueue
	}
	v, err := caseVideo(ctx, sqlDB, caseID, fileID)
	if err != nil {
		return err
	}
	if v.Status == "PENDING" || v.Status == "PROCESSING" {
		return ErrBusy
	}
	q := db.New(sqlDB)
	if err := q.ResetCaseVideo(ctx, fileID); err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]int64{"fileId": fileID})
	if _, err := queue.Enqueue(asynq.NewTask(TaskTypeProcess, payload), asynq.Timeout(30*time.Minute)); err != nil {
		err = fmt.Errorf("video: enqueue: %w", err)
		_ = q.FailCaseVideo(ctx, db.FailCaseVideoParams{CaseFileID: fileID, Error: sql.NullString{String: err.Error(), Valid: true}})
		return err
	}
	return nil
}
