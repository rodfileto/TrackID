// import-latentes-graph reads the LATENTES_SRMG CSV (the same file cmd/import-fingerprint-cases
// loads into Postgres) and writes its LTvsULF rows into Neo4j as evidence-to-evidence match
// decisions, per backend/internal/graph/migrations/002_forensic_evidence_capture.cypher.
//
// LTvsULF is a latent print compared against the unsolved-latent file: both sides of the row
// name a criminal case's evidence, not an identified person (see fingerprintcase.normalizeRow),
// which is exactly the Object:Evidence-to-Object:Evidence comparison that migration models. Every
// other comparison type (TPvsULF, LTvsTPF, ...) involves an identified person via InfoBio and is
// out of scope here.
//
// Simplifications, deliberate for this first pass:
//   - The full raw string on each side (e.g. "02.2005.01.SRMG.31010-001-01-01") is imported as one
//     Object:Evidence.evidenceId, with no attempt to split case number from evidence sub-item.
//   - The related reference is always a bare case number in this dataset (e.g.
//     "02.2005.01.SRMG.00008", never "...-001-01-01" — verified across all 1,146 LTvsULF rows),
//     so it never actually names the specific evidence item that matched. Until a future version
//     of this data carries the exact evidence number, defaultEvidenceSuffix is appended so both
//     sides of a comparison are shaped as evidence identifiers; see relatedEvidenceID.
//   - Every biometric feature is recorded as an unknown-finger-to-unknown-finger match: the CSV
//     does not say which finger was involved, so fingerPosition is always "UNKNOWN" rather than
//     guessed.
//
// Defaults to a dry run: it only parses the CSV and reports how many rows would be written. Pass
// -commit to actually write, which requires NEO4J_URL (and NEO4J_PASSWORD; NEO4J_USERNAME
// defaults to "neo4j") to be configured.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/rodrigorfcm/trackid/backend/internal/env"
	"github.com/rodrigorfcm/trackid/backend/internal/fingerprintcase"
)

const comparisonTypeLTvsULF = "LTvsULF"

// defaultEvidenceSuffix stands in for the evidence-item suffix (case number-NNN-NN-NN) that
// LTvsULF's related reference never carries in this dataset — it always stops at the bare case
// number. A future, more precise export is expected to carry the real suffix; evidenceSuffix
// already matches it and leaves it alone rather than double-appending.
const defaultEvidenceSuffix = "-001-01-01"

var evidenceSuffix = regexp.MustCompile(`-\d{3}-\d{2}-\d{2}$`)

// relatedEvidenceID shapes a related reference as an evidence identifier, appending
// defaultEvidenceSuffix only when the reference doesn't already look like one.
func relatedEvidenceID(reference string) string {
	if evidenceSuffix.MatchString(reference) {
		return reference
	}
	return reference + defaultEvidenceSuffix
}

// importQuery is one UNWIND-based write so the whole CSV is applied in a single transaction.
// Every MERGE keys on a business identifier, so re-running this against the same file is safe.
const importQuery = `
UNWIND $rows AS row
MERGE (evidenceA:Object:Evidence {evidenceId: row.evidenceId})
ON CREATE SET evidenceA.description = row.description, evidenceA.evidenceType = "LATENT_FINGERPRINT"
MERGE (evidenceB:Object:Evidence {evidenceId: row.relatedReference})
ON CREATE SET evidenceB.evidenceType = "LATENT_FINGERPRINT"

MERGE (featureA:Object:BiometricFeature:FingerprintLift {featureId: row.evidenceId + "#feature"})
ON CREATE SET featureA.featureType = "FINGERPRINT_LIFT", featureA.liftMethod = "UNKNOWN", featureA.fingerPosition = "UNKNOWN"
MERGE (evidenceA)-[:HAS_FEATURE]->(featureA)

MERGE (featureB:Object:BiometricFeature:FingerprintLift {featureId: row.relatedReference + "#feature"})
ON CREATE SET featureB.featureType = "FINGERPRINT_LIFT", featureB.liftMethod = "UNKNOWN", featureB.fingerPosition = "UNKNOWN"
MERGE (evidenceB)-[:HAS_FEATURE]->(featureB)

MERGE (decision:Event:Decision {decisionId: row.evidenceId + "#" + row.relatedReference})
ON CREATE SET decision.matchType = row.comparisonType, decision.source = "LATENTES_SRMG_IMPORT"
MERGE (evidenceA)-[:COMPARED]->(decision)
MERGE (decision)-[:COMPARED]->(evidenceB)

FOREACH (_ IN CASE WHEN row.responsibleUser IS NOT NULL THEN [1] ELSE [] END |
	MERGE (user:User {username: row.responsibleUser})
	MERGE (user)-[:DECIDED]->(decision)
)
RETURN count(row) AS imported
`

func main() {
	env.Load()

	path := flag.String("file", "data/LATENTES_SRMG.csv", "path to the LATENTES_SRMG CSV")
	commit := flag.Bool("commit", false, "actually write to Neo4j (default is a dry run that only reports counts)")
	flag.Parse()

	file, err := os.Open(*path)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	records, stats, err := fingerprintcase.Parse(file, 0)
	if err != nil {
		log.Fatalf("failed to parse %s: %v", *path, err)
	}
	log.Printf("parsed %d row(s) from %s (%d unparseable)", stats.Read, *path, len(stats.Errors))

	rows := selectLTvsULF(records)
	log.Printf("found %d %s comparison(s) to import", len(rows), comparisonTypeLTvsULF)
	if len(rows) == 0 {
		return
	}

	if !*commit {
		log.Printf("dry run: pass -commit to write %d evidence-to-evidence decision(s) to Neo4j", len(rows))
		return
	}

	neo4jURL := os.Getenv("NEO4J_URL")
	if neo4jURL == "" {
		log.Fatal("NEO4J_URL is not configured")
	}
	username := os.Getenv("NEO4J_USERNAME")
	if username == "" {
		username = "neo4j"
	}
	password := os.Getenv("NEO4J_PASSWORD")
	if password == "" {
		log.Fatal("NEO4J_PASSWORD is not configured")
	}

	ctx := context.Background()
	driver, err := neo4j.NewDriverWithContext(neo4jURL, neo4j.BasicAuth(username, password, ""))
	if err != nil {
		log.Fatal(err)
	}
	defer driver.Close(ctx)
	if err := driver.VerifyConnectivity(ctx); err != nil {
		log.Fatalf("could not connect to Neo4j at %s: %v", neo4jURL, err)
	}

	session := driver.NewSession(ctx, neo4j.SessionConfig{})
	defer session.Close(ctx)

	imported, err := session.ExecuteWrite(ctx, func(transaction neo4j.ManagedTransaction) (any, error) {
		result, err := transaction.Run(ctx, importQuery, map[string]any{"rows": rows})
		if err != nil {
			return nil, err
		}
		record, err := result.Single(ctx)
		if err != nil {
			return nil, err
		}
		value, _ := record.Get("imported")
		return value, nil
	})
	if err != nil {
		log.Fatalf("import failed: %v", err)
	}
	log.Printf("import complete: wrote %v decision(s)", imported)
}

// selectLTvsULF keeps only rows whose comparison type is LTvsULF and that name a related
// reference to compare against — matching fingerprintcase.normalizeRow's own criteria for
// setting RelatedReferenceKind to "CRIMINAL_CASE" (an LT* comparison always names another
// case/evidence, never an identified person).
func selectLTvsULF(records []fingerprintcase.Record) []map[string]any {
	var rows []map[string]any
	for _, record := range records {
		if record.ComparisonType == nil || !strings.EqualFold(*record.ComparisonType, comparisonTypeLTvsULF) {
			continue
		}
		if record.RelatedReference == nil {
			continue
		}
		var responsibleUser any
		if record.ResponsibleUser != nil {
			responsibleUser = *record.ResponsibleUser
		}
		rows = append(rows, map[string]any{
			"evidenceId":       record.CaseID,
			"description":      record.Description,
			"relatedReference": relatedEvidenceID(*record.RelatedReference),
			"comparisonType":   *record.ComparisonType,
			"responsibleUser":  responsibleUser,
		})
	}
	return rows
}
