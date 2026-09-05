// Package videoproc finds and groups faces across a video: sample frames,
// detect (and, on some frames, embed) faces in each, cluster the embeddings
// into one group per person, and assemble a track per group. It owns no
// HTTP or job-polling concerns -- Process runs synchronously and reports
// progress via a callback, so a caller can drive it from a goroutine.
package videoproc

import (
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	"math"
	"os"
	"sort"

	"trackid-backend/internal/vision"
)

type Options struct {
	IntervalSeconds         float64 // spacing, in seconds, between sampled frames
	EmbedIntervalSeconds    float64 // how often a sampled frame also gets embedded (must be >= IntervalSeconds)
	MinBlur                 float64 // discard detections whose crop's blur score falls below this
	FullDetectionEveryFrame bool    // embed every sampled frame instead of only every EmbedIntervalSeconds
	IoUThreshold            float64 // box overlap required for a detect-only frame to inherit a nearby embedded frame's cluster
	ClusterEPS              float64 // DBSCAN neighborhood radius, cosine distance
	ClusterMinSamples       int     // DBSCAN minimum neighborhood size to seed a cluster
}

// FaceDetection is one face found in one sampled frame.
type FaceDetection struct {
	FrameNumber      int
	TimestampSeconds float64
	Confidence       float32
	BlurScore        float64
	Box              [4]float32
	IsEmbedding      bool
	ClusterID        int // -1 means unclustered/noise
	FaceCrop         string

	embedding []float32 // only set when IsEmbedding; feeds clustering, not exposed to callers
}

// Track is every detection believed to belong to the same person.
type Track struct {
	TrackID  int
	BestFace FaceDetection
	AllFaces []FaceDetection
}

type Result struct {
	Tracks     []Track
	TrackCount int
}

type ProgressFunc func(frameCount, expectedFrames int)

// Process runs the full pipeline against a video file already on disk. It
// blocks until done, so callers wanting a "start it, poll it" job should
// run it in a goroutine.
func Process(ctx context.Context, vis *vision.Service, videoPath string, opts Options, onProgress ProgressFunc) (Result, error) {
	duration, err := probeDuration(ctx, videoPath)
	if err != nil {
		return Result{}, fmt.Errorf("probe video: %w", err)
	}
	expectedFrames := int(duration/opts.IntervalSeconds) + 1

	frameDir, framePaths, err := extractFrames(ctx, videoPath, opts.IntervalSeconds)
	if err != nil {
		return Result{}, fmt.Errorf("extract frames: %w", err)
	}
	defer os.RemoveAll(frameDir)

	embedEvery := int(math.Round(opts.EmbedIntervalSeconds / opts.IntervalSeconds))
	if embedEvery < 1 {
		embedEvery = 1
	}

	var all []FaceDetection
	for i, framePath := range framePaths {
		isEmbedFrame := opts.FullDetectionEveryFrame || i%embedEvery == 0

		frameImg, err := decodeImageFile(framePath)
		if err != nil {
			return Result{}, fmt.Errorf("decode frame %d: %w", i, err)
		}

		raw, err := detectFrame(vis, framePath, isEmbedFrame)
		if err != nil {
			return Result{}, fmt.Errorf("detect frame %d: %w", i, err)
		}

		timestamp := float64(i) * opts.IntervalSeconds
		for _, d := range raw {
			blur := blurScore(frameImg, d.box)
			if blur < opts.MinBlur {
				continue
			}
			crop, err := cropDataURI(frameImg, d.box)
			if err != nil {
				return Result{}, fmt.Errorf("crop frame %d: %w", i, err)
			}
			all = append(all, FaceDetection{
				FrameNumber:      i,
				TimestampSeconds: timestamp,
				Confidence:       d.score,
				BlurScore:        blur,
				Box:              d.box,
				IsEmbedding:      isEmbedFrame,
				ClusterID:        -1,
				FaceCrop:         crop,
				embedding:        d.embedding,
			})
		}

		if onProgress != nil {
			onProgress(i+1, expectedFrames)
		}
	}

	clusterEmbeddings(all, opts.ClusterEPS, opts.ClusterMinSamples)
	propagateClusters(all, opts.IoUThreshold)
	tracks := assembleTracks(all)

	return Result{Tracks: tracks, TrackCount: len(tracks)}, nil
}

type rawDetection struct {
	box       [4]float32
	score     float32
	embedding []float32 // nil unless this came from a full detect+embed pass
}

func detectFrame(vis *vision.Service, framePath string, embed bool) ([]rawDetection, error) {
	f, err := os.Open(framePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if embed {
		faces, err := vis.DetectAndEmbed(f)
		if err != nil {
			return nil, err
		}
		out := make([]rawDetection, len(faces))
		for i, face := range faces {
			out[i] = rawDetection{box: face.Box, score: face.Score, embedding: face.Embedding}
		}
		return out, nil
	}

	dets, err := vis.DetectFaces(f)
	if err != nil {
		return nil, err
	}
	out := make([]rawDetection, len(dets))
	for i, d := range dets {
		out[i] = rawDetection{box: d.Box, score: d.Score}
	}
	return out, nil
}

func decodeImageFile(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	return img, err
}

// clusterEmbeddings runs DBSCAN over every embedded detection and writes
// the resulting cluster id back onto each one.
func clusterEmbeddings(all []FaceDetection, eps float64, minSamples int) {
	var idx []int
	var embeddings [][]float32
	for i, d := range all {
		if d.IsEmbedding {
			idx = append(idx, i)
			embeddings = append(embeddings, d.embedding)
		}
	}
	if len(embeddings) == 0 {
		return
	}
	labels := dbscan(embeddings, eps, minSamples)
	for k, label := range labels {
		all[idx[k]].ClusterID = label
	}
}

// propagateClusters assigns every detect-only detection the cluster id of
// whichever embedded detection's box overlaps it most, so long as that
// overlap clears the threshold -- this is how the cheap per-frame boxes
// that never got an embedding still end up attributed to a person.
func propagateClusters(all []FaceDetection, iouThreshold float64) {
	for i := range all {
		if all[i].IsEmbedding {
			continue
		}
		bestCluster := -1
		bestIoU := float32(iouThreshold)
		for j := range all {
			if !all[j].IsEmbedding || all[j].ClusterID < 0 {
				continue
			}
			if iou := vision.IoU(all[i].Box, all[j].Box); iou > bestIoU {
				bestIoU = iou
				bestCluster = all[j].ClusterID
			}
		}
		all[i].ClusterID = bestCluster
	}
}

// assembleTracks groups every clustered detection (noise excluded) into one
// Track per cluster, sorted by descending confidence within the track.
func assembleTracks(all []FaceDetection) []Track {
	byCluster := map[int][]FaceDetection{}
	for _, d := range all {
		if d.ClusterID < 0 {
			continue
		}
		byCluster[d.ClusterID] = append(byCluster[d.ClusterID], d)
	}

	clusterIDs := make([]int, 0, len(byCluster))
	for id := range byCluster {
		clusterIDs = append(clusterIDs, id)
	}
	sort.Ints(clusterIDs)

	tracks := make([]Track, 0, len(clusterIDs))
	for trackID, cid := range clusterIDs {
		faces := byCluster[cid]
		sort.Slice(faces, func(a, b int) bool { return faces[a].Confidence > faces[b].Confidence })
		tracks = append(tracks, Track{
			TrackID:  trackID,
			BestFace: faces[0],
			AllFaces: faces,
		})
	}
	return tracks
}
