// match-threshold shows and sets the automatic matcher's threshold versions
// (match_thresholds; see biometricmatch.Band). With no -review/-confirm it
// lists the versions of -embedding-type, newest (current) first. Setting one
// records a new version, which the worker's next sync of that modality applies:
//
//	match-threshold -review 0.60 -confirm 0.70 -source "ROC on validation set X" -commit
//	match-threshold -embedding-type FINGERPRINT_SOURCEAFIS -review 40 -confirm 60 -source "..." -commit
//
// Cutoffs are on the scale of biometric_decisions.confidence: similarities
// (1 - cosine distance) for faces, SourceAFIS scores for fingerprints. Setting
// defaults to a dry run; pass -commit to write.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/rodfileto/trackid/biometricmatch"
	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/embedding"
	"github.com/rodfileto/trackid/fingerprint"
	"github.com/rodfileto/trackid/internal/cmdutil"
	"github.com/rodfileto/trackid/internal/env"
)

func main() {
	env.Load()

	embeddingType := flag.String("embedding-type", embedding.EmbeddingType, "feature_embeddings.embedding_type, or biometric_templates.template_type (e.g. "+fingerprint.TemplateType+"), the threshold applies to")
	review := flag.Float64("review", 0, "score at or above which a pair gets a SYSTEM INCONCLUSIVE, for an examiner to review (a similarity for faces; the matcher's own score for "+fingerprint.TemplateType+")")
	confirm := flag.Float64("confirm", 0, "score at or above which a pair gets a SYSTEM POSITIVE (CONFIRMED on its own)")
	source := flag.String("source", "", "where the numbers come from (required when setting)")
	createdBy := flag.String("by", os.Getenv("USER"), "who is setting it")
	commit := cmdutil.CommitFlag()
	flag.Parse()

	sqlDB := cmdutil.OpenDB()
	defer sqlDB.Close()
	ctx := context.Background()

	if *review == 0 && *confirm == 0 {
		rows, err := db.New(sqlDB).ListMatchThresholds(ctx, *embeddingType)
		cmdutil.Fatal(err)
		if len(rows) == 0 {
			d := embedding.DefaultFaceBand
			if *embeddingType == fingerprint.TemplateType {
				d = fingerprint.DefaultBand
			}
			fmt.Printf("%s: no version set; the worker uses its default, review %.3f confirm %.3f\n", *embeddingType, d.Review, d.Confirm)
			return
		}
		for i, r := range rows {
			current := ""
			if i == 0 {
				current = "  (current)"
			}
			fmt.Printf("#%d %s review %.3f confirm %.3f  %s  by %s at %s%s\n", r.ID, r.EmbeddingType, r.ReviewThreshold,
				r.ConfirmThreshold, r.Source, r.CreatedBy.String, r.CreatedAt.Format("2006-01-02 15:04"), current)
		}
		return
	}

	if *confirm == 0 {
		*confirm = *review
	}
	band := biometricmatch.Band{Review: *review, Confirm: *confirm}
	if *embeddingType == fingerprint.TemplateType {
		cmdutil.Fatal(band.Validate()) // SourceAFIS scores are unbounded above
	} else {
		cmdutil.Fatal(band.ValidateSimilarity())
	}
	if *source == "" {
		log.Fatal("-source is required: say where the numbers come from")
	}
	if !*commit {
		log.Printf("dry run: would set %s to review %.3f confirm %.3f (%s)", *embeddingType, band.Review, band.Confirm, *source)
		return
	}
	v, err := biometricmatch.SetBand(ctx, sqlDB, *embeddingType, band, *source, *createdBy)
	cmdutil.Fatal(err)
	log.Printf("set %s threshold version #%d: review %.3f confirm %.3f", v.EmbeddingType, v.ID, v.Band.Review, v.Band.Confirm)
}
