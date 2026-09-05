package handlers

import (
	"context"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"trackid-backend/internal/videoproc"
)

func resolveVideoOptions(c *gin.Context) videoproc.Options {
	queryFloat := func(key string, fallback float64) float64 {
		if v := c.Query(key); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				return f
			}
		}
		return fallback
	}
	queryInt := func(key string, fallback int) int {
		if v := c.Query(key); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				return n
			}
		}
		return fallback
	}
	queryBool := func(key string, fallback bool) bool {
		if v := c.Query(key); v != "" {
			if b, err := strconv.ParseBool(v); err == nil {
				return b
			}
		}
		return fallback
	}

	return videoproc.Options{
		IntervalSeconds:         queryFloat("interval_seconds", 0.5),
		EmbedIntervalSeconds:    queryFloat("embed_interval_seconds", 1.0),
		MinBlur:                 queryFloat("min_blur", 50.0),
		FullDetectionEveryFrame: queryBool("full_detection_every_frame", false),
		IoUThreshold:            queryFloat("iou_threshold", 0.15),
		ClusterEPS:              queryFloat("cluster_eps", 0.45),
		ClusterMinSamples:       queryInt("cluster_min_samples", 2),
	}
}

func (d Deps) processVideo(c *gin.Context) {
	opts := resolveVideoOptions(c)

	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "missing file field"})
		return
	}

	tmp, err := os.CreateTemp("", "trackid-upload-*.mp4")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "could not accept upload"})
		return
	}
	defer tmp.Close()

	src, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "could not read upload"})
		return
	}
	defer src.Close()

	if _, err := io.Copy(tmp, src); err != nil {
		os.Remove(tmp.Name())
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "could not save upload"})
		return
	}
	tmpPath := tmp.Name()

	jobID := uuid.NewString()
	d.Jobs.Create(jobID)

	go func() {
		defer os.Remove(tmpPath)
		result, err := videoproc.Process(context.Background(), d.Vision, tmpPath, opts, func(frameCount, expectedFrames int) {
			d.Jobs.UpdateProgress(jobID, frameCount, expectedFrames)
		})
		if err != nil {
			d.Jobs.Fail(jobID, err)
			return
		}
		d.Jobs.Complete(jobID, result)
	}()

	c.JSON(http.StatusOK, gin.H{"job_id": jobID})
}

func (d Deps) getVideoJob(c *gin.Context) {
	jobID := c.Param("job_id")
	snap, ok := d.Jobs.Snapshot(jobID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"detail": "job not found"})
		return
	}

	body := gin.H{
		"job_id":          jobID,
		"status":          string(snap.Status),
		"frame_count":     snap.FrameCount,
		"expected_frames": snap.ExpectedFrames,
		"percent":         percentOf(snap.FrameCount, snap.ExpectedFrames),
		"elapsed_seconds": snap.Elapsed.Seconds(),
		"eta_seconds":     nil,
		"error":           nil,
		"result":          nil,
	}
	if snap.ETA != nil {
		body["eta_seconds"] = snap.ETA.Seconds()
	}
	if snap.Err != nil {
		body["error"] = snap.Err.Error()
	}
	if snap.Status == "completed" {
		body["result"] = videoResultDTO(snap.Result)
	}

	c.JSON(http.StatusOK, body)
}

func percentOf(count, expected int) float64 {
	if expected <= 0 {
		return 0
	}
	pct := float64(count) / float64(expected) * 100
	if pct > 100 {
		return 100
	}
	return pct
}

func videoResultDTO(result videoproc.Result) gin.H {
	tracks := make([]gin.H, len(result.Tracks))
	for i, t := range result.Tracks {
		allFaces := make([]gin.H, len(t.AllFaces))
		for j, f := range t.AllFaces {
			allFaces[j] = faceDetectionDTO(f)
		}
		tracks[i] = gin.H{
			"track_id":  t.TrackID,
			"best_face": faceDetectionDTO(t.BestFace),
			"all_faces": allFaces,
		}
	}
	return gin.H{
		"tracks":      tracks,
		"track_count": result.TrackCount,
		"video_id":    nil,
	}
}

func faceDetectionDTO(f videoproc.FaceDetection) gin.H {
	var clusterID any
	if f.ClusterID >= 0 {
		clusterID = f.ClusterID
	}
	var faceCrop any
	if f.FaceCrop != "" {
		faceCrop = f.FaceCrop
	}

	return gin.H{
		"frame_number":      f.FrameNumber,
		"timestamp_seconds": f.TimestampSeconds,
		"confidence":        f.Confidence,
		"quality_score":     nil, // no quality model in the Go vision stack yet
		"blur_score":        f.BlurScore,
		"bbox":              []float32{f.Box[0], f.Box[1], f.Box[2], f.Box[3]},
		"is_embedding":      f.IsEmbedding,
		"estimated_age":     nil, // no age/gender model in the Go vision stack yet
		"estimated_gender":  nil,
		"cluster_id":        clusterID,
		"face_crop":         faceCrop,
	}
}
