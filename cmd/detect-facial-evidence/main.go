// detect-facial-evidence runs automatic face detection over every evidence image
// attached to a FACIAL criminal case and records what it finds through the same
// write path a human marking a trace by hand already goes through:
// cases.DetectFaces (the configured detector) proposes boxes, cases.CreateTraces persists each as a
// case_traces row ("a face record" once the detector has identified it), its
// QUESTIONED biometricfeature, and a placeholder case_codifications row -- then
// enqueues embedding computation for it (see MODEL.md section 2.2/6).
//
// This repo is the shared core other organizations import their own data into
// (see identity.Ingest/cases.Ingest's own doc comments); after an organization's
// import brings in new FACIAL evidence, this is the backfill that turns it into
// traces/codifications. It is therefore built to be re-run at any time, not just
// once: an evidence file that already has any case_traces (cases.ListTraces) is
// left untouched, so running it again after adding more cases/evidence only
// processes what's new. A file that fails detection (unsupported format, missing
// storage object, ...) is logged and skipped, not fatal -- one bad row shouldn't
// abort a run over thousands of others.
//
// Requires DATABASE_URL, S3_*, and VISION_DETECTOR_PATH/VISION_RECOGNIZER_PATH
// (there is no reason to run this without a working detector). REDIS_URL is
// optional but strongly recommended: without it, codifications are created but
// their embeddings are never computed (cmd/worker, running against Redis, is
// what actually computes them -- start it alongside/after this).
//
// Defaults to a dry run that only reports counts; pass -commit to write.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/rodfileto/trackid-vision/vision"

	"github.com/rodfileto/trackid/cases"
	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/embedding"
	"github.com/rodfileto/trackid/internal/cmdutil"
	"github.com/rodfileto/trackid/internal/config"
	"github.com/rodfileto/trackid/internal/env"
	"github.com/rodfileto/trackid/internal/queue"
	"github.com/rodfileto/trackid/storage"
)

type facialCase struct {
	id     int64
	caseID string
}

func main() {
	env.Load()

	commit := cmdutil.CommitFlag()
	flag.Parse()

	configuration := config.Load()
	sqlDB := cmdutil.OpenDB()
	defer sqlDB.Close()

	ctx := context.Background()

	facialCases, err := loadFacialCases(ctx, sqlDB)
	cmdutil.Fatal(err)

	if !*commit {
		pending, done := 0, 0
		for _, c := range facialCases {
			files, err := listEvidenceFileIDs(ctx, sqlDB, c.id)
			cmdutil.Fatal(err)
			for _, fileID := range files {
				existing, err := cases.ListTraces(ctx, sqlDB, c.caseID, fileID)
				cmdutil.Fatal(err)
				if len(existing) > 0 {
					done++
				} else {
					pending++
				}
			}
		}
		log.Printf("dry run: %d FACIAL case(s); %d evidence file(s) already have traces, %d pending detection",
			len(facialCases), done, pending)
		return
	}

	if configuration.S3Endpoint == "" || configuration.S3AccessKey == "" || configuration.S3SecretKey == "" || configuration.S3Bucket == "" {
		log.Fatal("S3_ENDPOINT/S3_ACCESS_KEY/S3_SECRET_KEY/S3_BUCKET are required to download evidence images")
	}
	storeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	store, err := storage.NewClient(storeCtx, configuration.S3Endpoint, configuration.S3AccessKey, configuration.S3SecretKey, configuration.S3Bucket)
	cancel()
	cmdutil.Fatal(err)

	if configuration.VisionDetectorPath == "" || configuration.VisionRecognizerPath == "" {
		log.Fatal("VISION_DETECTOR_PATH/VISION_RECOGNIZER_PATH are required to run face detection")
	}
	vis, err := vision.NewService(vision.Config{
		Detector:          vision.DetectorKind(configuration.VisionDetector),
		DetectorPath:      configuration.VisionDetectorPath,
		RecognizerPath:    configuration.VisionRecognizerPath,
		SharedLibraryPath: configuration.VisionSharedLibraryPath,
		UseGPU:            configuration.VisionUseGPU,
	})
	cmdutil.Fatal(err)
	defer vis.Close()

	// Left as a nil embedding.Enqueuer (not a typed-nil *asynq.Client stored in
	// it) when Redis isn't configured -- cases.CreateTraces' own `if queue !=
	// nil` guard only works against a genuinely nil interface value.
	var enqueuer embedding.Enqueuer
	if configuration.RedisURL != "" {
		queueClient, err := queue.OpenClient(configuration.RedisURL)
		if err != nil {
			log.Printf("Redis is configured but the queue client could not be created; embeddings will not be enqueued: %v", err)
		} else {
			defer queueClient.Close()
			enqueuer = queueClient
		}
	} else {
		log.Printf("REDIS_URL is not configured; codifications will be created but their embeddings will never be computed until it is")
	}

	casesDone, filesSkipped, filesProcessed, filesFailed, tracesCreated := 0, 0, 0, 0, 0
	for _, c := range facialCases {
		fileIDs, err := listEvidenceFileIDs(ctx, sqlDB, c.id)
		cmdutil.Fatal(err)

		for _, fileID := range fileIDs {
			existing, err := cases.ListTraces(ctx, sqlDB, c.caseID, fileID)
			cmdutil.Fatal(err)
			if len(existing) > 0 {
				filesSkipped++
				continue
			}

			proposals, err := cases.DetectFaces(ctx, sqlDB, store, vis, c.caseID, fileID)
			if err != nil {
				log.Printf("case %s evidence %d: detect faces: %v", c.caseID, fileID, err)
				filesFailed++
				continue
			}
			filesProcessed++
			if len(proposals) == 0 {
				continue
			}

			detections := make([]cases.TraceDetection, len(proposals))
			for i, p := range proposals {
				detections[i] = cases.TraceDetection{BoxX1: p.BoxX1, BoxY1: p.BoxY1, BoxX2: p.BoxX2, BoxY2: p.BoxY2, Score: p.Score}
			}
			traces, err := cases.CreateTraces(ctx, sqlDB, enqueuer, c.caseID, fileID, detections)
			if err != nil {
				log.Printf("case %s evidence %d: create traces: %v", c.caseID, fileID, err)
				filesFailed++
				continue
			}
			tracesCreated += len(traces)
		}

		casesDone++
		if casesDone%100 == 0 {
			log.Printf("progress: %d/%d cases, %d file(s) processed, %d skipped (already done), %d failed, %d trace(s) created",
				casesDone, len(facialCases), filesProcessed, filesSkipped, filesFailed, tracesCreated)
		}
	}
	log.Printf("detect-facial-evidence complete: %d case(s), %d file(s) processed, %d skipped (already done), %d failed, %d trace(s) created",
		casesDone, filesProcessed, filesSkipped, filesFailed, tracesCreated)
}

func loadFacialCases(ctx context.Context, sqlDB *sql.DB) ([]facialCase, error) {
	rows, err := sqlDB.QueryContext(ctx, `SELECT id, case_id FROM criminal_cases WHERE case_type = 'FACIAL' ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("detect-facial-evidence: query criminal_cases: %w", err)
	}
	defer rows.Close()

	var out []facialCase
	for rows.Next() {
		var c facialCase
		if err := rows.Scan(&c.id, &c.caseID); err != nil {
			return nil, fmt.Errorf("detect-facial-evidence: scan criminal_cases: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// listEvidenceFileIDs returns the id of every "evidence"-category case_files row
// for a case, via the same generated query the case-detail API uses.
func listEvidenceFileIDs(ctx context.Context, sqlDB *sql.DB, criminalCaseID int64) ([]int64, error) {
	q := db.New(sqlDB)
	rows, err := q.ListCaseFilesByCriminalCase(ctx, criminalCaseID)
	if err != nil {
		return nil, fmt.Errorf("detect-facial-evidence: list case_files for case %d: %w", criminalCaseID, err)
	}
	var ids []int64
	for _, row := range rows {
		if row.Category == "evidence" {
			ids = append(ids, row.ID)
		}
	}
	return ids, nil
}
