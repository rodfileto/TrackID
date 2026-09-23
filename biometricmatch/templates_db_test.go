package biometricmatch_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rodfileto/trackid/biometricmatch"
	"github.com/rodfileto/trackid/cases"
	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/fingerprint"
	"github.com/rodfileto/trackid/identity"
)

// fingerprintSidecar connects to the sourceafis-sidecar named by
// FINGERPRINT_SIDECAR_URL (e.g. http://localhost:58090), skipping the test
// when it's unset or unreachable -- this is an integration test against the
// real service, not a fake.
func fingerprintSidecar(t *testing.T) *fingerprint.Client {
	t.Helper()
	url := os.Getenv("FINGERPRINT_SIDECAR_URL")
	if url == "" {
		t.Skip("FINGERPRINT_SIDECAR_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := fingerprint.NewClient(ctx, url)
	if err != nil {
		t.Skipf("sourceafis-sidecar unreachable: %v", err)
	}
	return client
}

// TestRunTemplatesFingerprintPair is the "done when" scenario from
// trackid-sim's roadmap: a rolled print (KNOWN, enrolled) and a latent
// (QUESTIONED, lifted on a case) of the same generated finger, extracted
// through the real sidecar and matched through RunTemplates, become one
// SYSTEM POSITIVE biometric_decisions row.
func TestRunTemplatesFingerprintPair(t *testing.T) {
	ctx := context.Background()
	sidecar := fingerprintSidecar(t)
	sqlDB := testDB(t)

	rolled := readTestdata(t, "f000_r0.png")
	latent := readTestdata(t, "f000_l0.png")
	otherRolled := readTestdata(t, "f001_r0.png")

	// KNOWN: an enrolled ten-print, feature type FINGERPRINT_TEMPLATE.
	enrollRes, err := identity.Ingest(ctx, sqlDB, identity.Enrollment{
		PersonID:       "PERSON-FP-1",
		DocumentType:   "ID",
		DocumentNumber: "DOC-FP-1",
		RegisterNumber: "REG-FP-1",
		Name:           "Fingerprint Test Subject",
		Parent1Name:    "Parent One",
		Parent1Gender:  "U",
		Parent2Name:    "Parent Two",
		Parent2Gender:  "U",
		Files: []identity.FileInput{
			{FileType: "nist", StorageRef: "unused/rolled"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var knownFeatureID int64
	for _, f := range enrollRes.Features {
		if f.FileType == "nist" {
			knownFeatureID = f.FeatureID
		}
	}
	if knownFeatureID == 0 {
		t.Fatal("no FINGERPRINT_TEMPLATE biometricfeature produced by identity.Ingest")
	}

	// A second enrolled ten-print (a different finger), so RunTemplates has
	// an impostor pair available too -- not scored above threshold, and
	// exercised here mainly to prove genuine pairing isn't a coincidence of
	// there being only one candidate.
	otherEnrollRes, err := identity.Ingest(ctx, sqlDB, identity.Enrollment{
		PersonID:       "PERSON-FP-2",
		DocumentType:   "ID",
		DocumentNumber: "DOC-FP-2",
		RegisterNumber: "REG-FP-2",
		Name:           "Fingerprint Test Subject Two",
		Parent1Name:    "Parent One",
		Parent1Gender:  "U",
		Parent2Name:    "Parent Two",
		Parent2Gender:  "U",
		Files: []identity.FileInput{
			{FileType: "nist", StorageRef: "unused/rolled2"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var otherKnownFeatureID int64
	for _, f := range otherEnrollRes.Features {
		if f.FileType == "nist" {
			otherKnownFeatureID = f.FeatureID
		}
	}

	// QUESTIONED: a fingerprint lift on a case, feature type FINGERPRINT_LIFT.
	caseRes, err := cases.Ingest(ctx, sqlDB, cases.CaseInput{
		CaseID:   "CASE-FP-1",
		CaseType: cases.CaseTypeCriminal,
		Modality: "FINGERPRINT",
		Evidences: []cases.EvidenceInput{
			{
				Sequence: 1,
				Traces: []cases.TraceInput{
					{Sequence: 1, TraceType: "FINGERPRINT_LIFT"},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	questionedFeatureID := caseRes.Evidences[0].Traces[0].FeatureID
	if questionedFeatureID == 0 {
		t.Fatal("no FINGERPRINT_LIFT biometricfeature produced by cases.Ingest")
	}

	// Extract real templates through the sidecar and store them, mirroring
	// what fingerprint.ExtractForIdentityFeature/ExtractForCodification do
	// once they've resolved a source image -- this test skips the storage
	// download step and goes straight from testdata bytes to the sidecar.
	rolledTemplate, err := sidecar.Extract(ctx, rolled)
	if err != nil {
		t.Fatalf("extract rolled: %v", err)
	}
	latentTemplate, err := sidecar.Extract(ctx, latent)
	if err != nil {
		t.Fatalf("extract latent: %v", err)
	}
	otherRolledTemplate, err := sidecar.Extract(ctx, otherRolled)
	if err != nil {
		t.Fatalf("extract other rolled: %v", err)
	}

	q := db.New(sqlDB)
	for _, tpl := range []struct {
		featureID int64
		template  []byte
	}{
		{knownFeatureID, rolledTemplate},
		{questionedFeatureID, latentTemplate},
		{otherKnownFeatureID, otherRolledTemplate},
	} {
		if _, err := q.UpsertBiometricTemplate(ctx, db.UpsertBiometricTemplateParams{
			BiometricfeatureID: tpl.featureID,
			TemplateType:       fingerprint.TemplateType,
			Template:           tpl.template,
			ModelVersion:       sql.NullString{String: fingerprint.ModelVersion, Valid: true},
		}); err != nil {
			t.Fatal(err)
		}
	}

	stats, err := biometricmatch.RunTemplates(ctx, sqlDB, sidecar, fingerprint.TemplateType, fingerprint.MatchThreshold, fingerprint.TemplateType)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Decisions != 1 {
		t.Fatalf("Decisions = %d, want 1 (the rolled/latent genuine pair; the impostor pair should score below threshold)", stats.Decisions)
	}

	rows, err := sqlDB.QueryContext(ctx, `SELECT feature_a_id, feature_b_id, role, decision, system_source, confidence, threshold FROM biometric_decisions`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var a, b, role, decision, source string
		var confidence, threshold float64
		if err := rows.Scan(&a, &b, &role, &decision, &source, &confidence, &threshold); err != nil {
			t.Fatal(err)
		}
		count++
		if role != "SYSTEM" || decision != "POSITIVE" {
			t.Errorf("decision = (%s, %s), want (SYSTEM, POSITIVE)", role, decision)
		}
		if source != fingerprint.TemplateType {
			t.Errorf("system_source = %q, want %q", source, fingerprint.TemplateType)
		}
		if threshold != fingerprint.MatchThreshold {
			t.Errorf("threshold = %v, want %v", threshold, fingerprint.MatchThreshold)
		}
		if confidence < fingerprint.MatchThreshold {
			t.Errorf("confidence = %v, want >= threshold %v", confidence, fingerprint.MatchThreshold)
		}
		t.Logf("decision: %s <-> %s, score %.2f", a, b, confidence)
	}
	if count != 1 {
		t.Fatalf("biometric_decisions rows = %d, want 1", count)
	}
}

func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "fingerprint", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}
