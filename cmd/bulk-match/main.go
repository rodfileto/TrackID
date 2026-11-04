// bulk-match matches, calibrates and applies thresholds over a large gallery loaded by
// bulk-enroll, in batches (docs/large-gallery-plan.md). Three subcommands:
//
//	bulk-match match -size 2000000 -shards /data/shards -commit
//	bulk-match calibrate
//	bulk-match apply -commit
//
// match takes a batch of BULK embeddings (at random, by -seed), and the GPU matcher
// (tools/bulk-match, FAISS) compares it exactly against itself, every earlier batch and every
// trace. Per face it keeps the neighbours scoring at least -floor, at most -k, in the score store
// (bulk_match_scores), and marks the batch matched. A batch left SELECTED by a failed run is
// resumed by the next.
//
// calibrate reports, from the stored scores and the identity records, the genuine and impostor
// curves, examiners' decisions on stored pairs, and per threshold the review load and the
// projected mean false degree. It writes nothing. Choosing a threshold version stays a separate
// act, with cmd/match-threshold.
//
// apply bands the stored scores under the current threshold version, writes the SYSTEM decisions
// that changed (citing the version), clusters once, and reports the clusters and the review
// queue: pairs at or above review whose records say different people.
//
// The cycle: match a pilot batch, calibrate, set a version, apply; examiners review the queue;
// calibrate again with their decisions, set a new version, apply (which re-bands everything
// stored, without matching again); match the next batch; and so on.
//
// match and apply default to a dry run; pass -commit to write.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/rodfileto/trackid/bulk"
	"github.com/rodfileto/trackid/embedding"
	"github.com/rodfileto/trackid/internal/cmdutil"
	"github.com/rodfileto/trackid/internal/env"
)

const defaultMatcher = "uv run --project tools/bulk-match --extra gpu python tools/bulk-match/bulk_match.py"

func usage() {
	fmt.Fprintf(os.Stderr, "usage: bulk-match match|calibrate|apply [flags]\n  bulk-match <subcommand> -h for its flags\n")
	os.Exit(2)
}

func main() {
	env.Load()
	if len(os.Args) < 2 {
		usage()
	}
	sub, args := os.Args[1], os.Args[2:]
	fs := flag.NewFlagSet("bulk-match "+sub, flag.ExitOnError)
	embeddingType := fs.String("embedding-type", embedding.EmbeddingType, "feature_embeddings.embedding_type to match")
	modelVersion := fs.String("model-version", embedding.ModelVersion, "model version: batches, scores and calibration are per model")
	ctx := context.Background()

	switch sub {
	case "match":
		size := fs.Int("size", 0, "faces in a new batch (required unless a SELECTED batch is resumed)")
		seed := fs.String("seed", "1", "seed of the batch's random selection")
		floor := fs.Float64("floor", 0.3, "lowest score kept in the score store, well below any plausible review threshold")
		k := fs.Int("k", 100, "most neighbours kept per face; faces that fill it are recorded as hubs")
		shards := fs.String("shards", "", "vector shard directory written by bulk-enroll (required)")
		work := fs.String("work", "", "work directory for the matcher's files (default <shards>/work/batch-<id>)")
		matcher := fs.String("matcher", defaultMatcher, "command running the GPU matcher; --work <dir> is appended")
		gpu := fs.String("gpu", "auto", `matcher device: "auto", "cpu", or device ids ("0,1")`)
		commit := fs.Bool("commit", false, "actually write (default is a dry run that only reports counts)")
		fs.Parse(args)
		if *shards == "" {
			log.Fatal("-shards is required")
		}
		sqlDB := cmdutil.OpenDB()
		defer sqlDB.Close()
		r, err := bulk.Match(ctx, sqlDB, bulk.MatchOptions{
			EmbeddingType: *embeddingType, ModelVersion: *modelVersion, Size: *size, Seed: *seed, Floor: *floor, K: *k,
			ShardsDir: *shards, WorkDir: *work, Matcher: strings.Fields(*matcher), GPU: *gpu, Commit: *commit,
		})
		cmdutil.Fatal(err)
		if !*commit {
			if r.Resumed {
				log.Printf("dry run: batch %d (%d faces) is SELECTED and would be resumed", r.Batch, r.Size)
			} else {
				log.Printf("dry run: %d BULK embeddings wait for a batch; a new batch would take %d", r.Pending, min(int64(*size), r.Pending))
			}
			return
		}
		resumed := ""
		if r.Resumed {
			resumed = " (resumed)"
		}
		log.Printf("batch %d%s: %d faces against %d earlier and %d traces; %.4g comparisons on %s in %.1f s; %d scores stored, %d hubs; %d shard rows filled from Postgres",
			r.Batch, resumed, r.Size, r.Earlier, r.Others, float64(r.Comparisons), r.Matcher, r.Seconds, r.Scores, r.Hubs, r.Filled)

	case "calibrate":
		key := fs.String("identity-key", strings.Join(bulk.DefaultIdentityKey, ","), "record fields deciding same and different identity, in order (fiscal, name_birth, person)")
		maxGroup := fs.Int("max-group", 100, "ignore identity keys shared by more faces than this")
		finalSize := fs.Int64("final-size", 0, "gallery size to project the mean false degree to (default: every KNOWN embedding of the model)")
		step := fs.Float64("step", 0.01, "threshold grid spacing")
		jsonOut := fs.String("json", "", "also write the report as JSON to this file")
		fs.Parse(args)
		identityKey, err := bulk.ParseIdentityKey(*key)
		cmdutil.Fatal(err)
		sqlDB := cmdutil.OpenDB()
		defer sqlDB.Close()
		c, err := bulk.Calibrate(ctx, sqlDB, bulk.CalibrateOptions{EmbeddingType: *embeddingType, ModelVersion: *modelVersion,
			IdentityKey: identityKey, MaxGroup: *maxGroup, FinalSize: *finalSize, Step: *step})
		cmdutil.Fatal(err)
		c.Print(os.Stdout)
		if *jsonOut != "" {
			data, _ := json.MarshalIndent(c, "", "  ")
			cmdutil.Fatal(os.WriteFile(*jsonOut, append(data, '\n'), 0o644))
		}

	case "apply":
		key := fs.String("identity-key", strings.Join(bulk.DefaultIdentityKey, ","), "record fields deciding same and different identity, in order (fiscal, name_birth, person)")
		allowDefault := fs.Bool("allow-default", false, "apply the built-in default band when no threshold version is set")
		chunk := fs.Int64("chunk", 200_000, "embedding ids per write transaction")
		queue := fs.Int("queue", 20, "review-queue pairs to print")
		queueOut := fs.String("queue-out", "", "write the whole review queue to this CSV file")
		maxCluster := fs.Int("max-cluster", 50, "alarm (exit status 3) when the largest automatic cluster is bigger")
		commit := fs.Bool("commit", false, "actually write (default is a dry run that only reports counts)")
		fs.Parse(args)
		identityKey, err := bulk.ParseIdentityKey(*key)
		cmdutil.Fatal(err)
		sqlDB := cmdutil.OpenDB()
		defer sqlDB.Close()
		r, err := bulk.Apply(ctx, sqlDB, bulk.ApplyOptions{EmbeddingType: *embeddingType, ModelVersion: *modelVersion,
			Fallback: embedding.DefaultFaceBand, AllowDefault: *allowDefault, IdentityKey: identityKey, Chunk: *chunk,
			Queue: *queue, QueueOut: *queueOut, MaxCluster: *maxCluster, Commit: *commit})
		cmdutil.Fatal(err)
		r.Print(os.Stdout, *commit)
		if r.Alarm {
			os.Exit(3)
		}

	default:
		usage()
	}
}
