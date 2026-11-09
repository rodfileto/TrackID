// calibrate-face-threshold measures how the face model's scores separate the same person from different people, over
// enrolled faces whose identity a deployment trusts (calibration.IdentitySource), and suggests the threshold for a
// false match rate. It reads and writes nothing else; setting a threshold stays a separate act, cmd/match-threshold.
//
//	calibrate-face-threshold -key person                          # persons.person_id marks one person
//	calibrate-face-threshold -key person -document-types RIN      # only these documents' faces (PF: InfoBio's,
//	                                                              # whose persons are fingerprint-deduplicated)
//	calibrate-face-threshold -key fiscal,name_birth -identities 10000
//
// Which field says two faces are one person is the deployment's choice, the whole calibration rests on it: a key
// that joins different people (a placeholder fiscal number, a common name) makes impostors look genuine, and one that
// splits a person makes the same person look like two.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"

	"github.com/rodfileto/trackid/calibration"
	"github.com/rodfileto/trackid/embedding"
	"github.com/rodfileto/trackid/internal/cmdutil"
	"github.com/rodfileto/trackid/internal/env"
)

func main() {
	env.Load()
	embeddingType := flag.String("embedding-type", embedding.EmbeddingType, "feature_embeddings.embedding_type to calibrate")
	keyFlag := flag.String("key", "", "the identity fields that mark one person, in order: person, fiscal, name_birth (required)")
	documentTypes := flag.String("document-types", "", "only faces of these identity_documents.document_type, comma-separated; all when empty")
	identities := flag.Int("identities", 3000, "identities with two faces or more to sample")
	maxFaces := flag.Int("max-faces", 5, "faces kept per identity")
	maxImpostor := flag.Int("max-impostor-pairs", 5_000_000, "impostor pairs scored at most")
	seed := flag.Int64("seed", 1, "the sample's seed")
	flag.Parse()
	key, err := calibration.ParseKey(*keyFlag)
	if err != nil {
		log.Fatalf("%v: say which field marks one person with -key", err)
	}
	var types []string
	for _, t := range strings.Split(*documentTypes, ",") {
		if t = strings.TrimSpace(t); t != "" {
			types = append(types, t)
		}
	}

	sqlDB := cmdutil.OpenDB()
	defer sqlDB.Close()
	source := calibration.IdentitySource{DB: sqlDB, EmbeddingType: *embeddingType, Key: key, DocumentTypes: types}
	report, err := calibration.Run(context.Background(), source, calibration.Options{
		Identities: *identities, MaxFaces: *maxFaces, MaxImpostorPairs: *maxImpostor, Seed: *seed,
	})
	cmdutil.Fatal(err)
	fmt.Print(report)
	for _, t := range report.Targets {
		if t.Reached {
			fmt.Printf("\nto set it (FMR %.0e), with a review band below for examiners if wanted:\n", t.FMR)
			fmt.Printf("  match-threshold -embedding-type %s -review %.2f -confirm %.2f -source %q -commit\n",
				*embeddingType, t.Threshold, t.Threshold, fmt.Sprintf("calibrate-face-threshold: %s, seed %d, %d genuine / %d impostor pairs, FMR %.0e",
					source.Describe(), *seed, report.GenuinePairs, report.ImpostorPairs, t.FMR))
			break
		}
	}
}
