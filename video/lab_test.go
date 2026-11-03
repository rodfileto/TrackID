package video

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rodfileto/trackid-vision/videoproc"
)

func TestLabPersonsOrdersFacesAndMarksBest(t *testing.T) {
	best := videoproc.FaceDetection{FrameNumber: 2, TimestampSeconds: 2, Confidence: 0.95, Box: [4]float32{10, 10, 50, 50}, IsEmbedding: true}
	result := videoproc.Result{Tracks: []videoproc.Track{{
		TrackID:  0,
		BestFace: best,
		// videoproc sorts by confidence; the lab shows them in time order.
		AllFaces: []videoproc.FaceDetection{best,
			{FrameNumber: 4, TimestampSeconds: 4, Confidence: 0.9, Box: [4]float32{12, 10, 52, 50}},
			{FrameNumber: 0, TimestampSeconds: 0, Confidence: 0.8, Box: [4]float32{8, 10, 48, 50}, IsEmbedding: true}},
	}}, TrackCount: 1}
	persons := LabPersons(result, 25)
	if len(persons) != 1 {
		t.Fatalf("persons = %d", len(persons))
	}
	p := persons[0]
	if len(p.Faces) != 3 || p.Faces[0].TimestampMs != 0 || p.Faces[1].TimestampMs != 2000 || p.Faces[2].TimestampMs != 4000 {
		t.Fatalf("faces not in time order: %+v", p.Faces)
	}
	nBest := 0
	for _, f := range p.Faces {
		if f.Best {
			nBest++
		}
	}
	if nBest != 1 || !p.Faces[1].Best || !p.Best.Best || p.Best.TimestampMs != 2000 {
		t.Fatalf("best face not marked once: %+v", p)
	}
	if p.Faces[1].Frame == nil || *p.Faces[1].Frame != 50 {
		t.Fatalf("frame number at 2 s and 25 fps = %v, want 50", p.Faces[1].Frame)
	}
	if p.FirstMs != 0 || p.LastMs != 4000 {
		t.Fatalf("span = %d..%d", p.FirstMs, p.LastMs)
	}
	if LabPersons(result, 0)[0].Faces[0].Frame != nil {
		t.Fatal("frame numbers without a frame rate")
	}
}

func TestLabOptionsBounds(t *testing.T) {
	o, err := LabOptions{}.Normalized()
	if err != nil || o.IntervalSeconds != LabDefaultInterval || o.MinBlur != LabDefaultMinBlur {
		t.Fatalf("defaults: %+v %v", o, err)
	}
	for _, bad := range []LabOptions{{IntervalSeconds: 0.01}, {IntervalSeconds: 60}, {MinBlur: -1}} {
		if _, err := bad.Normalized(); err == nil {
			t.Fatalf("%+v accepted", bad)
		}
	}
}

// testVideo makes a 3-second 10 fps clip with ffmpeg, or skips without it.
func testVideo(t *testing.T) []byte {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not on PATH")
	}
	path := filepath.Join(t.TempDir(), "clip.mp4")
	out, err := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc=size=160x120:rate=10:duration=3",
		"-pix_fmt", "yuv420p", path).CombinedOutput()
	if err != nil {
		t.Fatalf("ffmpeg: %v: %s", err, out)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func waitJob(t *testing.T, lab *Lab, id string) LabJob {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		j, err := lab.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		if j.Status == "DONE" || j.Status == "FAILED" {
			return j
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the job did not finish")
	return LabJob{}
}

func TestLabProcessesAndExtractsFrames(t *testing.T) {
	data := testVideo(t)
	var got LabOptions
	tracker := func(ctx context.Context, path string, opts LabOptions, onProgress videoproc.ProgressFunc) (videoproc.Result, error) {
		got = opts
		onProgress(1, 2)
		onProgress(2, 2)
		face := videoproc.FaceDetection{FrameNumber: 1, TimestampSeconds: 1, Confidence: 0.9, Box: [4]float32{1, 2, 30, 40}, IsEmbedding: true}
		return videoproc.Result{Tracks: []videoproc.Track{{TrackID: 0, BestFace: face, AllFaces: []videoproc.FaceDetection{face}}}, TrackCount: 1}, nil
	}
	lab := NewLab(tracker)
	defer lab.Close()
	job, err := lab.Add("clip.mp4", bytes.NewReader(data), 10<<20, LabOptions{IntervalSeconds: 0.5})
	if err != nil {
		t.Fatal(err)
	}
	if len(job.SHA256) != 64 || job.SizeBytes != int64(len(data)) {
		t.Fatalf("job = %+v", job)
	}
	done := waitJob(t, lab, job.ID)
	if done.Status != "DONE" || len(done.Persons) != 1 || got.IntervalSeconds != 0.5 || got.MinBlur != LabDefaultMinBlur {
		t.Fatalf("done = %+v, opts = %+v", done, got)
	}
	if done.FrameRate != 10 || done.DurationSeconds < 2.9 || done.Persons[0].Faces[0].Frame == nil || *done.Persons[0].Faces[0].Frame != 10 {
		t.Fatalf("probe or frame numbers: %+v", done)
	}
	png, err := lab.Frame(context.Background(), job.ID, 1500)
	if err != nil || !bytes.HasPrefix(png, []byte("\x89PNG")) {
		t.Fatalf("frame: %v (%d bytes)", err, len(png))
	}
	if _, err := lab.Frame(context.Background(), job.ID, 60_000); !errors.Is(err, ErrOutOfRange) {
		t.Fatalf("frame past the end: %v", err)
	}
	if err := lab.Remove(job.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := lab.Get(job.ID); !errors.Is(err, ErrLabJobNotFound) {
		t.Fatalf("after remove: %v", err)
	}
}

func TestLabRefusesAndFails(t *testing.T) {
	if _, err := NewLab(nil).Add("clip.mp4", strings.NewReader("x"), 10, LabOptions{}); !errors.Is(err, ErrLabNoTracker) {
		t.Fatalf("no tracker: %v", err)
	}
	failing := func(context.Context, string, LabOptions, videoproc.ProgressFunc) (videoproc.Result, error) {
		return videoproc.Result{}, errors.New("detector broke")
	}
	lab := NewLab(failing)
	defer lab.Close()
	if _, err := lab.Add("notes.txt", strings.NewReader("x"), 10, LabOptions{}); !errors.Is(err, ErrNotVideo) {
		t.Fatalf("not a video: %v", err)
	}
	if _, err := lab.Add("big.mp4", strings.NewReader("0123456789AB"), 10, LabOptions{}); err == nil {
		t.Fatal("a video over the limit was taken")
	}
	// Bytes that are not a video fail at the probe, recorded on the job.
	job, err := lab.Add("fake.mp4", strings.NewReader("not a video at all"), 1<<20, LabOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if done := waitJob(t, lab, job.ID); done.Status != "FAILED" || done.Error == "" {
		t.Fatalf("garbage video: %+v", done)
	}
	data := testVideo(t)
	job, err = lab.Add("clip.mp4", bytes.NewReader(data), 10<<20, LabOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if done := waitJob(t, lab, job.ID); done.Status != "FAILED" || done.Error != "detector broke" {
		t.Fatalf("tracker failure: %+v", done)
	}
}

func TestLabEvictsFinishedJobsOnly(t *testing.T) {
	block := make(chan struct{})
	tracker := func(ctx context.Context, _ string, _ LabOptions, _ videoproc.ProgressFunc) (videoproc.Result, error) {
		select {
		case <-block:
		case <-ctx.Done():
		}
		return videoproc.Result{}, ctx.Err()
	}
	data := testVideo(t)
	lab := NewLab(tracker)
	defer lab.Close()
	defer close(block)
	for i := 0; i < LabMaxJobs; i++ {
		if _, err := lab.Add("clip.mp4", bytes.NewReader(data), 10<<20, LabOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := lab.Add("clip.mp4", bytes.NewReader(data), 10<<20, LabOptions{}); !errors.Is(err, ErrLabBusy) {
		t.Fatalf("a fifth job while all run: %v", err)
	}
}
