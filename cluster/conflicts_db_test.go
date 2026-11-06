package cluster_test

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"testing"

	. "github.com/rodfileto/trackid/cluster"
)

func TestSameName(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{"Ana Silva", "Ana Silva", true},
		{"ANA SILVA", "ana silva", true},
		{"José Araújo", "Jose Araujo", true},
		{"Gabriel Silva Pereira", "Gabriel Sillva Pereira", true},
		{"Katia Fraga das Oliveira", "Katai Fraga das Oliveira", true},
		{"Luis Demetrius Ricardo Silvestre", "Lui Demetrius Ricardo Silvestre", true},
		{"Ana S. Silva", "Ana Silva", true},
		{"Ivaldo Lima Alves", "Volnei Miguel Cruz Silva", false},
		{"Ana Silva", "Ana Souza", false},
		{"Maria Silva", "Maria Souza Silva", false},
	} {
		if got := SameName(tc.a, tc.b); got != tc.want {
			t.Errorf("SameName(%q, %q) = %t", tc.a, tc.b, got)
		}
	}
}

// conflictWorld enrolls people and files for the conflict tests. Each record is one identity file,
// whose id is the feature id.
type conflictWorld struct {
	t   *testing.T
	db  *sql.DB
	seq int
}

func (w *conflictWorld) exec(statement string, args ...any) {
	w.t.Helper()
	if _, err := w.db.Exec(statement, args...); err != nil {
		w.t.Fatalf("%s: %v", statement, err)
	}
}

func (w *conflictWorld) person(number int64) {
	w.exec(`INSERT INTO persons (id, person_id) VALUES ($1, $2)`, number, fmt.Sprintf("SIM-P-%d", number))
}

// record adds an enrollment with a photo and a print, with the given file ids.
func (w *conflictWorld) record(person int64, name string, photo, print int64) {
	w.t.Helper()
	w.seq++
	w.exec(`INSERT INTO identity_documents (id, person_id, document_number, document_type) VALUES ($1, $2, $3, 'RG')`, w.seq, person, fmt.Sprintf("D%d", w.seq))
	w.exec(`INSERT INTO identity_registers (id, document_id, register_number, name, parent_1_name, parent_1_gender, parent_2_name, parent_2_gender, birth_date)
		VALUES ($1, $1, $2, $3, 'M', 'F', 'P', 'M', '1980-01-01')`, w.seq, fmt.Sprintf("R%d", w.seq), name)
	w.exec(`INSERT INTO identity_files (id, register_id, file_type, source_path, storage_ref) VALUES ($1, $2, 'photo', 'a', 'a'), ($3, $2, 'nist', 'b', 'b')`, photo, w.seq, print)
}

func (w *conflictWorld) cluster(id int64, basis, modality string, features ...string) {
	w.t.Helper()
	w.exec(`INSERT INTO clusters (id, modality, basis) VALUES ($1, $2, $3)`, id, modality, basis)
	for _, f := range features {
		w.exec(`INSERT INTO cluster_members (cluster_id, basis, feature_id) VALUES ($1, $2, $3)`, id, basis, f)
	}
}

func (w *conflictWorld) matcher(a, b, modality string, score, threshold float64) {
	w.t.Helper()
	w.exec(`INSERT INTO biometric_decisions (feature_a_id, feature_b_id, modality, role, decision, system_source, confidence, threshold)
		VALUES ($1, $2, $3, 'SYSTEM', 'POSITIVE', 'test', $4, $5)`, a, b, modality, score, threshold)
}

func (w *conflictWorld) examiners(a, b, modality, decision string) {
	w.t.Helper()
	w.exec(`INSERT INTO biometric_decisions (feature_a_id, feature_b_id, modality, role, decision, username) VALUES
		($1, $2, $3, 'VERIFICATOR', $4, 'one'), ($1, $2, $3, 'REVIEWER', $4, 'two')`, a, b, modality, decision)
}

// embed gives a file an embedding pointing along one axis, so two files on different axes score 0.
func (w *conflictWorld) embed(file int64, axis int) {
	w.t.Helper()
	values := make([]string, 512)
	for i := range values {
		values[i] = "0"
	}
	values[axis] = "1"
	w.exec(`WITH f AS (INSERT INTO biometric_features (feature_type, provenance, identity_file_id) VALUES ('FACE_RECORD', 'KNOWN', $1) RETURNING id)
		INSERT INTO feature_embeddings (biometric_feature_id, embedding_type, embedding) SELECT id, 'FACE_AURAFACE_512', $2::vector FROM f`,
		file, "["+strings.Join(values, ",")+"]")
}

func findConflict(conflicts []Conflict, kind ConflictKind, person int64) *Conflict {
	for i := range conflicts {
		if conflicts[i].Kind == kind && slices.ContainsFunc(conflicts[i].Persons, func(p ConflictPerson) bool { return p.PersonNumber == person }) {
			return &conflicts[i]
		}
	}
	return nil
}

func TestListConflicts(t *testing.T) {
	ctx := context.Background()
	sqlDB := testDB(t)
	w := &conflictWorld{t: t, db: sqlDB}
	for n := int64(1); n <= 11; n++ {
		w.person(n)
	}
	const a, r = "AUTOMATIC", "REVIEWED"

	// The impostor: person 1's second record holds person 2's biometrics. It raises both kinds.
	w.record(1, "Ana Silva", 1, 2)
	w.record(1, "Ana Silva", 3, 4)
	w.record(2, "Bruno Costa", 5, 6)
	w.cluster(1, a, "FACIAL", "1")
	w.cluster(2, a, "FACIAL", "3", "5")
	w.cluster(3, a, "FINGERPRINT", "2")
	w.cluster(4, a, "FINGERPRINT", "4", "6")
	w.matcher("3", "5", "FACIAL", 0.99, 0.7)
	w.matcher("4", "6", "FINGERPRINT", 300, 60)
	w.embed(1, 0)
	w.embed(3, 1)

	// Normal, and not to be flagged: two documents and one name in one cluster; two person rows with
	// one name; a misspelling on a renewal.
	w.record(3, "Carla Dias", 7, 8)
	w.record(3, "Carla Dias", 9, 10)
	w.cluster(5, a, "FACIAL", "7", "9")
	w.cluster(6, a, "FINGERPRINT", "8", "10")
	w.record(4, "Dora Lima", 11, 12)
	w.record(5, "Dora Lima", 13, 14)
	w.cluster(7, a, "FACIAL", "11", "13")
	w.record(6, "Gabriel Silva Pereira", 15, 16)
	w.record(6, "Gabriel Sillva Pereira", 17, 18)
	w.cluster(8, a, "FACIAL", "15", "17")

	// One modality only: weak.
	w.record(7, "Elisa Melo", 19, 20)
	w.record(8, "Fabio Nunes", 21, 22)
	w.cluster(9, a, "FACIAL", "19", "21")
	w.matcher("19", "21", "FACIAL", 0.8, 0.7)

	// A chain: nothing compares these two directly.
	w.record(9, "Gil Prado", 23, 24)
	w.record(10, "Hugo Reis", 25, 26)
	w.cluster(10, a, "FACIAL", "23", "25")

	// Examiners looked, and it is not the same person.
	w.record(11, "Iara Sousa", 27, 28)
	w.record(11, "Julia Teles", 29, 30)
	w.cluster(11, a, "FACIAL", "27", "29")
	w.examiners("27", "29", "FACIAL", "NEGATIVE")

	conflicts, err := ListConflicts(ctx, sqlDB)
	if err != nil {
		t.Fatal(err)
	}

	names := findConflict(conflicts, ConflictManyNames, 2)
	if names == nil || names.Strength != StrengthTwoModalities || len(names.Persons) != 2 ||
		!slices.Equal(names.ClusterIDs, []int64{2, 4}) || !slices.Equal(names.Modalities, []string{"FACIAL", "FINGERPRINT"}) {
		t.Fatalf("one biometric, many names: %+v", names)
	}
	if len(names.Comparisons) != 2 || names.Comparisons[0].A.Name == names.Comparisons[0].B.Name || *names.Comparisons[0].Score != 0.99 || *names.Comparisons[0].Threshold != 0.7 {
		t.Errorf("comparisons: %+v", names.Comparisons)
	}
	bodies := findConflict(conflicts, ConflictManyBiometrics, 1)
	if bodies == nil || bodies.Strength != StrengthTwoModalities || !slices.Equal(bodies.ClusterIDs, []int64{1, 2, 3, 4}) {
		t.Fatalf("one identity, many biometrics: %+v", bodies)
	}
	// The faces are measured apart; the prints have nothing on record, and do not pretend otherwise.
	facial := bodies.Comparisons[slices.IndexFunc(bodies.Comparisons, func(c ConflictComparison) bool { return c.Modality == "FACIAL" })]
	if facial.Score == nil || *facial.Score > 0.01 || !facial.Measured {
		t.Errorf("measured face: %+v", facial)
	}
	print := bodies.Comparisons[slices.IndexFunc(bodies.Comparisons, func(c ConflictComparison) bool { return c.Modality == "FINGERPRINT" })]
	if print.Score != nil || print.Measured || print.Status != NoDecision {
		t.Errorf("fingerprint with no comparison: %+v", print)
	}

	// Normal cases are not conflicts, in either direction.
	for _, person := range []int64{3, 4, 5, 6} {
		if c := ConflictsOfPerson(conflicts, person); len(c) != 0 {
			t.Errorf("person %d, who is normal, is flagged: %+v", person, c)
		}
	}
	if c := findConflict(conflicts, ConflictManyNames, 7); c == nil || c.Strength != StrengthOneModality {
		t.Errorf("one modality: %+v", c)
	}
	if c := findConflict(conflicts, ConflictManyNames, 9); c == nil || c.Strength != StrengthIndirect || len(c.Comparisons) != 0 {
		t.Errorf("a chain: %+v", c)
	}
	// One person with two truly different names in one cluster is a conflict too, here rejected.
	if c := findConflict(conflicts, ConflictManyNames, 11); c == nil || c.Strength != StrengthRejected {
		t.Errorf("rejected: %+v", c)
	}
	// Strongest first, the rejected last.
	if conflicts[0].Strength != StrengthTwoModalities || conflicts[len(conflicts)-1].Strength != StrengthRejected {
		t.Errorf("order: %s ... %s", conflicts[0].Strength, conflicts[len(conflicts)-1].Strength)
	}
	if got := ConflictsOfCluster(conflicts, 2); len(got) != 2 {
		t.Errorf("conflicts of cluster 2: %+v", got)
	}

	// The reviewed layer shows what people have confirmed, and cannot raise a split: nobody has
	// reviewed person 3's records, which sit alone there.
	w.cluster(20, r, "FACIAL", "7")
	w.cluster(21, r, "FACIAL", "9")
	w.examiners("3", "5", "FACIAL", "POSITIVE")
	w.cluster(22, r, "FACIAL", "3", "5")
	// A trace confirmed as person 2: their status, in the person page's sense.
	w.exec(`INSERT INTO biometric_cases (id, case_id, case_type, modality, description) VALUES (1, 'C-1', 'CRIMINAL', 'FACIAL', 'd')`)
	w.exec(`INSERT INTO case_evidences (id, biometric_case_id, sequence) VALUES (1, 1, 1)`)
	w.exec(`INSERT INTO case_traces (id, evidence_id, sequence, trace_type) VALUES (1, 1, 1, 'FACE_CAPTURE')`)
	w.exec(`INSERT INTO cluster_members (cluster_id, basis, feature_id) VALUES (2, 'AUTOMATIC', 'TRACE:1#feature'), (22, 'REVIEWED', 'TRACE:1#feature')`)
	w.examiners("5", "TRACE:1#feature", "FACIAL", "POSITIVE")

	conflicts, err = ListConflicts(ctx, sqlDB)
	if err != nil {
		t.Fatal(err)
	}
	if c := ConflictsOfPerson(conflicts, 3); len(c) != 0 {
		t.Errorf("the reviewed layer raised a split: %+v", c)
	}
	names = findConflict(conflicts, ConflictManyNames, 2)
	if names == nil || names.Strength != StrengthReviewed || !slices.Equal(names.ReviewedClusterIDs, []int64{22}) || names.Traces != 1 {
		t.Fatalf("reviewed: %+v", names)
	}
	var ana, bruno *ConflictPerson
	for i := range names.Persons {
		if names.Persons[i].PersonNumber == 1 {
			ana = &names.Persons[i]
		} else {
			bruno = &names.Persons[i]
		}
	}
	if ana.Status != nil || bruno.Status == nil || bruno.Status.Basis != LinkLead {
		t.Errorf("status: Ana %+v, Bruno %+v", ana.Status, bruno.Status)
	}
	if got := ConflictsOfCluster(conflicts, 22); len(got) != 1 {
		t.Errorf("a reviewed cluster's conflicts: %+v", got)
	}

	// Parents' names only come when a page asks for them.
	if names.Registers != nil {
		t.Errorf("registers without asking: %+v", names.Registers)
	}
	flagged := ConflictsOfPerson(conflicts, 2)
	if err := AddRegisters(ctx, sqlDB, flagged); err != nil {
		t.Fatal(err)
	}
	withRegisters := findConflict(flagged, ConflictManyNames, 2)
	if len(withRegisters.Registers) != 3 || withRegisters.Registers[0].Parent1Name != "M" || withRegisters.Registers[0].BirthDate != "1980-01-01" {
		t.Errorf("registers: %+v", withRegisters.Registers)
	}

	// The listing marks them, and can keep only them.
	overview, err := ListOverview(ctx, sqlDB, ListOverviewParams{PageSize: 100})
	if err != nil {
		t.Fatal(err)
	}
	marked := 0
	for _, s := range overview.Items {
		if len(s.Conflicts) > 0 {
			marked++
		}
	}
	only, err := ListOverview(ctx, sqlDB, ListOverviewParams{PageSize: 100, ConflictOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if marked == 0 || only.Total != marked || len(only.Items) != marked || overview.Total <= only.Total {
		t.Errorf("listing: %d marked, %d of %d kept", marked, only.Total, overview.Total)
	}
}

// TestConflictsAcrossPersonRows: nothing merges people, so one identity can be several person rows. A
// fake document is found by what it copies, whatever row the importer gave it.
func TestConflictsAcrossPersonRows(t *testing.T) {
	ctx := context.Background()
	sqlDB := testDB(t)
	w := &conflictWorld{t: t, db: sqlDB}
	for n := int64(1); n <= 12; n++ {
		w.person(n)
	}
	const a = "AUTOMATIC"
	fiscal := func(person int64, number string) {
		w.exec(`UPDATE identity_documents SET fiscal_number = $2 WHERE person_id = $1`, person, number)
	}
	birth := func(person int64, date string) {
		w.exec(`UPDATE identity_registers SET birth_date = $2 WHERE document_id IN (SELECT id FROM identity_documents WHERE person_id = $1)`, person, date)
	}

	// A CNH on another person row with the victim's CPF and name, and biometrics of its own: the fraudster
	// has no other record, so no cluster holds two names.
	w.record(1, "Ana Silva", 1, 2)
	w.record(2, "Ana Silva", 3, 4)
	fiscal(1, "CPF-1")
	fiscal(2, "CPF-1")
	w.cluster(1, a, "FACIAL", "1")
	w.cluster(2, a, "FACIAL", "3")
	w.cluster(3, a, "FINGERPRINT", "2")
	w.cluster(4, a, "FINGERPRINT", "4")

	// The same with no CPF on the fake: only the name and birth date bind them.
	w.record(3, "Bia Lima", 5, 6)
	w.record(4, "Bia Lima", 7, 8)
	fiscal(3, "CPF-3")
	birth(3, "1991-01-01")
	birth(4, "1991-01-01")
	w.cluster(5, a, "FACIAL", "5")
	w.cluster(6, a, "FACIAL", "7")

	// Homonyms, by their different CPFs: two people, not one.
	w.record(5, "Caio Reis", 9, 10)
	w.record(6, "Caio Reis", 11, 12)
	fiscal(5, "CPF-5")
	fiscal(6, "CPF-6")
	birth(5, "1992-02-02")
	birth(6, "1992-02-02")
	w.cluster(7, a, "FACIAL", "9")
	w.cluster(8, a, "FACIAL", "11")

	// A CPF-less record that could belong to either of two people stays apart from both.
	w.record(7, "Davi Paz", 13, 14)
	w.record(8, "Davi Paz", 15, 16)
	w.record(9, "Davi Paz", 17, 18)
	fiscal(7, "CPF-7")
	fiscal(8, "CPF-8")
	birth(7, "1993-03-03")
	birth(8, "1993-03-03")
	birth(9, "1993-03-03")
	w.cluster(9, a, "FACIAL", "13")
	w.cluster(10, a, "FACIAL", "15")
	w.cluster(11, a, "FACIAL", "17")

	// A genuine RG from another state: its own row, the same CPF and one biometric. Normal.
	w.record(10, "Eva Luz", 19, 20)
	w.record(11, "Eva Luz", 21, 22)
	fiscal(10, "CPF-10")
	fiscal(11, "CPF-10")
	w.cluster(12, a, "FACIAL", "19", "21")

	conflicts, err := ListConflicts(ctx, sqlDB)
	if err != nil {
		t.Fatal(err)
	}
	cpf := findConflict(conflicts, ConflictManyBiometrics, 2)
	if cpf == nil || cpf.IdentityBasis != "FISCAL" || cpf.Strength != StrengthTwoModalities || len(cpf.Persons) != 2 || !slices.Equal(cpf.ClusterIDs, []int64{1, 2, 3, 4}) {
		t.Fatalf("same CPF on two rows: %+v", cpf)
	}
	if c := findConflict(conflicts, ConflictManyBiometrics, 4); c == nil || c.IdentityBasis != "NAME_BIRTH" || c.Strength != StrengthOneModality || len(c.Persons) != 2 {
		t.Errorf("same name and birth with no CPF: %+v", c)
	}
	// Neither of these fraudsters' clusters holds two names, so the other kind says nothing.
	if c := findConflict(conflicts, ConflictManyNames, 2); c != nil {
		t.Errorf("no cluster holds two names here: %+v", c)
	}
	for _, person := range []int64{5, 6, 7, 8, 9, 10, 11} {
		if c := ConflictsOfPerson(conflicts, person); len(c) != 0 {
			t.Errorf("person %d is flagged: %+v", person, c)
		}
	}
	// The comparison table shows the CPFs, which are what bind the records.
	flagged := ConflictsOfPerson(conflicts, 2)
	if err := AddRegisters(ctx, sqlDB, flagged); err != nil {
		t.Fatal(err)
	}
	if r := flagged[0].Registers; len(r) != 2 || r[0].FiscalNumber != "CPF-1" {
		t.Errorf("registers: %+v", r)
	}
}
