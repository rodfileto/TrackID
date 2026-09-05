package videoproc

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// probeDuration returns a video's duration in seconds via ffprobe.
func probeDuration(ctx context.Context, videoPath string) (float64, error) {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "csv=p=0",
		videoPath,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	duration, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		return 0, fmt.Errorf("ffprobe: unexpected duration output %q: %w", out, err)
	}
	return duration, nil
}

// extractFrames samples the video at fixed time intervals into JPEG files
// under a fresh temp directory, one file per sampled frame. The returned
// slice is sorted, one path per frame in order.
func extractFrames(ctx context.Context, videoPath string, intervalSeconds float64) (dir string, frames []string, err error) {
	dir, err = os.MkdirTemp("", "trackid-frames-*")
	if err != nil {
		return "", nil, fmt.Errorf("create frame dir: %w", err)
	}

	pattern := filepath.Join(dir, "frame_%06d.jpg")
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner", "-loglevel", "error", "-y",
		"-i", videoPath,
		"-vf", fmt.Sprintf("fps=1/%f", intervalSeconds),
		"-qmin", "1", "-qmax", "1", "-q:v", "1",
		pattern,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		os.RemoveAll(dir)
		return "", nil, fmt.Errorf("ffmpeg: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		os.RemoveAll(dir)
		return "", nil, fmt.Errorf("read frame dir: %w", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".jpg") {
			frames = append(frames, filepath.Join(dir, e.Name()))
		}
	}
	// os.ReadDir already returns entries sorted by filename, and the
	// zero-padded %06d naming keeps that in frame order.
	return dir, frames, nil
}
