package cluster_test

import (
	"context"
	"testing"

	"github.com/rodfileto/trackid/cluster"
	"github.com/rodfileto/trackid/graph"
)

// A person enrolled with a face and a ten-print: the face's cluster lists both samples and links
// the fingerprint cluster (by enrollment, in the same layer), and the fingerprint's links back.
func TestDetailLinksOtherModality(t *testing.T) {
	ctx := context.Background()
	sqlDB := testDB(t)
	exec := func(query string, args ...any) int64 {
		t.Helper()
		var id int64
		if err := sqlDB.QueryRowContext(ctx, query+" RETURNING id", args...).Scan(&id); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		return id
	}
	person := exec(`INSERT INTO persons (person_id) VALUES ('P-1')`)
	doc := exec(`INSERT INTO identity_documents (person_id, document_number, document_type) VALUES ($1, 'RG-1', 'RG')`, person)
	register := exec(`INSERT INTO identity_registers (document_id, register_number, name, parent_1_name, parent_1_gender, parent_2_name, parent_2_gender)
		VALUES ($1, 'R-1', 'JOAO', 'X', 'M', 'Y', 'F')`, doc)
	known := func(fileType, featureType string) string {
		file := exec(`INSERT INTO identity_files (register_id, file_type, sequence, source_path, storage_ref)
			VALUES ($1, $2, 1, 'x', 'x')`, register, fileType)
		exec(`INSERT INTO biometric_features (feature_type, provenance, identity_file_id) VALUES ($1, 'KNOWN', $2)`, featureType, file)
		return graph.KnownFeatureID(file)
	}
	photo, nist := known("photo", "FACE_RECORD"), known("nist", "FINGERPRINT_TEMPLATE")

	bc := exec(`INSERT INTO biometric_cases (case_id, case_type, modality, description) VALUES ('2026.0001', 'CRIMINAL', 'FINGERPRINT', 'burglary')`)
	evidence := exec(`INSERT INTO case_evidences (biometric_case_id, sequence) VALUES ($1, 1)`, bc)
	trace := exec(`INSERT INTO case_traces (evidence_id, sequence, trace_type) VALUES ($1, 1, 'FINGERPRINT_LIFT')`, evidence)
	exec(`INSERT INTO biometric_features (feature_type, provenance, case_trace_id) VALUES ('FINGERPRINT_LIFT', 'QUESTIONED', $1)`, trace)

	put := func(modality string, features ...string) int64 {
		t.Helper()
		id := exec(`INSERT INTO clusters (modality, basis) VALUES ($1, 'AUTOMATIC')`, modality)
		for _, f := range features {
			if _, err := sqlDB.ExecContext(ctx, `INSERT INTO cluster_members (cluster_id, basis, feature_id) VALUES ($1, 'AUTOMATIC', $2)`, id, f); err != nil {
				t.Fatal(err)
			}
		}
		return id
	}
	facial := put("FACIAL", photo)
	fingerprint := put("FINGERPRINT", nist, graph.QuestionedFeatureID(trace))

	d, err := cluster.GetDetail(ctx, sqlDB, facial)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.PersonSamples) != 2 {
		t.Fatalf("person samples: %+v", d.PersonSamples)
	}
	modalities := map[string]string{}
	for _, s := range d.PersonSamples {
		modalities[s.FeatureID] = s.Modality
	}
	if modalities[photo] != "FACIAL" || modalities[nist] != "FINGERPRINT" {
		t.Errorf("sample modalities: %v", modalities)
	}
	if len(d.LinkedClusters) != 1 {
		t.Fatalf("linked clusters: %+v", d.LinkedClusters)
	}
	if l := d.LinkedClusters[0]; l.ClusterID != fingerprint || l.Modality != "FINGERPRINT" || l.MemberCount != 2 ||
		l.QuestionedCount != 1 || len(l.PersonIDs) != 1 || l.PersonIDs[0] != "P-1" {
		t.Errorf("linked cluster: %+v", l)
	}

	d, err = cluster.GetDetail(ctx, sqlDB, fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.LinkedClusters) != 1 || d.LinkedClusters[0].ClusterID != facial || d.LinkedClusters[0].QuestionedCount != 0 {
		t.Errorf("linked back: %+v", d.LinkedClusters)
	}
}
