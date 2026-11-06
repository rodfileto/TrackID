package ask

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
)

// seedWalk adds people to seed's scenario.
//
//	person 1  Ana Silva, also Ana S. Silva   F-1 reviewed (and automatic), P-1 automatic only
//	person 2  Ana Silva, another one         nothing
//	person 3  Bruno Costa                    F-1 and P-1 automatic only
//
// F-1 belongs to occurrence 1, which shares inquiry IPL-7 with occurrence 2; P-1 has no occurrence.
// Occurrence 6 is known only to the municipality, close to occurrence 1.
func seedWalk(t *testing.T, sqlDB *sql.DB) {
	t.Helper()
	for _, statement := range []string{
		`INSERT INTO persons (id, person_id) VALUES (1, 'SIM-P-1'), (2, 'SIM-P-2'), (3, 'SIM-P-3')`,
		`INSERT INTO identity_documents (id, person_id, document_number, document_type) VALUES
			(1, 1, 'D1', 'RG'), (2, 2, 'D2', 'RG'), (3, 3, 'D3', 'RG')`,
		`INSERT INTO identity_registers (id, document_id, register_number, name, parent_1_name, parent_1_gender, parent_2_name, parent_2_gender, birth_date) VALUES
			(1, 1, 'R1', 'Ana Silva', 'Mother One', 'F', 'Father One', 'M', '1980-01-01'),
			(2, 1, 'R2', 'Ana S. Silva', 'Mother One', 'F', 'Father One', 'M', '1980-01-01'),
			(3, 2, 'R3', 'Ana Silva', 'Mother Two', 'F', 'Father Two', 'M', '1999-09-09'),
			(4, 3, 'R4', 'Bruno Costa', 'Mother Three', 'F', 'Father Three', 'M', '1970-07-07')`,
		`INSERT INTO identity_files (id, register_id, file_type, source_path, storage_ref) VALUES
			(1, 1, 'photo', 'a', 'a'), (2, 3, 'photo', 'b', 'b'), (3, 4, 'photo', 'c', 'c')`,
		`INSERT INTO case_evidences (id, biometric_case_id, sequence)
			SELECT bc.id, bc.id, 1 FROM biometric_cases bc`,
		`INSERT INTO case_traces (id, evidence_id, sequence, trace_type)
			SELECT ce.id, ce.id, 1, 'FACE_CAPTURE' FROM case_evidences ce`,
		`INSERT INTO clusters (id, modality, basis) VALUES (1, 'FACIAL', 'AUTOMATIC'), (2, 'FACIAL', 'REVIEWED')`,
		`INSERT INTO cluster_members (cluster_id, basis, feature_id)
			SELECT 1, 'AUTOMATIC', 'TRACE:' || ct.id || '#feature' FROM case_traces ct`,
		`INSERT INTO cluster_members (cluster_id, basis, feature_id) VALUES (1, 'AUTOMATIC', '1'), (1, 'AUTOMATIC', '3'), (2, 'REVIEWED', '1')`,
		`INSERT INTO cluster_members (cluster_id, basis, feature_id)
			SELECT 2, 'REVIEWED', 'TRACE:' || ct.id || '#feature' FROM case_traces ct
			JOIN case_evidences ce ON ce.id = ct.evidence_id JOIN biometric_cases bc ON bc.id = ce.biometric_case_id WHERE bc.case_id = 'F-1'`,
		`UPDATE occurrences SET inquiry_number = 'IPL-7' WHERE id IN (1, 2)`,
		`INSERT INTO locations (id, location_id, geom, precision, municipality_code, uf) VALUES
			(3, 'L3', ST_SetSRID(ST_MakePoint(-43.171, -22.901), 4326)::geography, 'MUNICIPALITY', '3304557', 'RJ')`,
		`INSERT INTO occurrences (id, agency, occurrence_number, location_id, occurred_from, registered_at, status, description) VALUES
			(6, 'DP-003', 'A6', 3, '2024-06-01 12:00-03', NOW(), 'REGISTERED', 'SIMULATED six')`,
		`INSERT INTO occurrence_classifications (occurrence_id, type_code) VALUES (6, 'PHONE_ROBBERY')`,
	} {
		if _, err := sqlDB.Exec(statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
}

func accessLogged(t *testing.T, sqlDB *sql.DB, kind, subject string) int {
	t.Helper()
	var n int
	if err := sqlDB.QueryRow(`SELECT COUNT(*) FROM access_log WHERE kind = $1 AND subject = $2 AND route LIKE '/api/ask#%'`, kind, subject).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestWalk(t *testing.T) {
	ctx := context.Background()
	sqlDB := testDB(t)
	asker := seed(t, sqlDB)
	seedWalk(t, sqlDB)
	s := NewService(sqlDB, nil)
	tax, err := s.loadTaxonomy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	call := func(tool, arguments string) any {
		t.Helper()
		value, err := s.runTool(ctx, asker, tax, toolCall{Name: tool, Arguments: arguments})
		if err != nil {
			t.Fatalf("%s %s: %v", tool, arguments, err)
		}
		return value
	}

	// Two people share the name: both come back, apart, one holding two spellings, and nobody's
	// history is logged by a search.
	found := call("search_persons", `{"name":"ana silva"}`).(PersonSearch)
	if found.RowCount != 2 {
		t.Fatalf("search: %+v", found)
	}
	byNumber := map[int64]FoundPerson{}
	for _, p := range found.Rows {
		byNumber[p.PersonNumber] = p
	}
	if p := byNumber[1]; p.Cases != 2 || p.ReviewedCases != 1 || p.BirthDates[0] != "1980-01-01" || p.Documents[0].Number != "D1" {
		t.Errorf("person 1: %+v", p)
	}
	if p := byNumber[2]; p.Cases != 0 || p.BirthDates[0] != "1999-09-09" {
		t.Errorf("person 2: %+v", p)
	}
	if got := accessLogged(t, sqlDB, accessPerson, "SIM-P-1"); got != 0 {
		t.Errorf("a search logged %d person reads", got)
	}
	if none := call("search_persons", `{"name":"%%"}`).(PersonSearch); none.RowCount != 0 {
		t.Errorf("a wildcard found people: %+v", none)
	}
	if encoded, _ := json.Marshal(found); strings.Contains(string(encoded), "Mother") {
		t.Errorf("a parent's name reached the model: %s", encoded)
	}

	// A person's cases carry their link, the occurrence, its inquiry and its place, and the read is logged.
	cases := call("person_cases", `{"person_number":1}`).(PersonCases)
	if cases.RowCount != 2 || cases.ReviewedCount != 1 || cases.LeadCount != 1 {
		t.Fatalf("person cases: %+v", cases)
	}
	if c := cases.Rows[0]; c.CaseID != "F-1" || c.Link != linkReviewed || c.Occurrence == nil || c.Occurrence.InquiryNumber != "IPL-7" ||
		c.Occurrence.Lat == nil || c.Occurrence.Precision != "EXACT" || c.Occurrence.OccurredFrom != "2024-01-10 12:00" {
		t.Errorf("reviewed case: %+v, occurrence %+v", c, c.Occurrence)
	}
	if c := cases.Rows[1]; c.CaseID != "P-1" || c.Link != linkLead || c.Occurrence != nil {
		t.Errorf("lead case: %+v", c)
	}
	if got := accessLogged(t, sqlDB, accessPerson, "SIM-P-1"); got != 1 {
		t.Errorf("person read logged %d times", got)
	}
	// The other Ana is another person: her walk ends at once.
	if other := call("person_cases", `{"person_number":2}`).(PersonCases); other.RowCount != 0 {
		t.Errorf("person 2 has cases: %+v", other)
	}
	// Entities are drawn from the result, for the next question.
	refs := map[string]bool{}
	for _, e := range cases.entities() {
		refs[e.Kind+":"+e.Ref] = true
	}
	for _, want := range []string{"PERSON:1", "CASE:F-1", "CASE:P-1", "OCCURRENCE:1", "INQUIRY:IPL-7"} {
		if !refs[want] {
			t.Errorf("entities lack %s: %v", want, refs)
		}
	}

	// A case: who else is linked, with each link, and the read logged.
	detail := call("case_detail", `{"case_id":"F-1"}`).(CaseDetail)
	if len(detail.Persons) != 2 || detail.Persons[0].PersonNumber != 1 || detail.Persons[0].Link != linkReviewed ||
		detail.Persons[1].PersonNumber != 3 || detail.Persons[1].Link != linkLead || detail.Occurrence == nil {
		t.Errorf("case detail: %+v", detail)
	}
	if got := accessLogged(t, sqlDB, accessCase, "F-1"); got != 1 {
		t.Errorf("case read logged %d times", got)
	}

	// Occurrences: found by number, read in full, and an inquiry is every occurrence with its number.
	search := call("find_occurrence", `{"query":"IPL-7"}`).(OccurrenceSearch)
	if search.RowCount != 2 {
		t.Errorf("find_occurrence: %+v", search)
	}
	occurrence := call("occurrence_detail", `{"occurrence_id":1}`).(OccurrenceResult).Occurrence
	if occurrence.Municipality != "Niterói" || occurrence.Neighbourhood != "Centro" || occurrence.PlaceType != "COMMERCE" ||
		len(occurrence.Cases) != 1 || len(occurrence.Cases[0].Persons) != 2 {
		t.Errorf("occurrence: %+v", occurrence)
	}
	if got := accessLogged(t, sqlDB, accessCase, "F-1"); got != 2 {
		t.Errorf("after occurrence_detail, the case read is logged %d times", got)
	}
	inquiry := call("inquiry_detail", `{"inquiry_number":"ipl-7"}`).(InquiryResult)
	if inquiry.RowCount != 2 {
		t.Errorf("inquiry: %+v", inquiry)
	}

	// Holding one of a case's analyses, reading it is the work: nothing more is logged.
	if _, err := sqlDB.Exec(`WITH a AS (INSERT INTO case_analyses (biometric_case_id, sequence, analysis_type)
			SELECT id, 1, 'FACIAL_REVIEW' FROM biometric_cases WHERE case_id = 'F-1' RETURNING id)
		INSERT INTO analysis_assignments (case_analysis_id, step, user_id, method) SELECT id, 'REVIEWER', $1, 'AUTOMATIC' FROM a`, asker.ID); err != nil {
		t.Fatal(err)
	}
	before := accessLogged(t, sqlDB, accessCase, "F-1")
	call("case_detail", `{"case_id":"F-1"}`)
	if got := accessLogged(t, sqlDB, accessCase, "F-1"); got != before {
		t.Errorf("a case the analyst holds was logged: %d, then %d", before, got)
	}

	// Near an occurrence: itself left out, a municipality's centre not treated as a place.
	near := call("occurrences_near", `{"occurrence_id":1,"radius_m":5000}`).(OccurrencesNear)
	ids := []int64{}
	for _, o := range near.Rows {
		ids = append(ids, o.ID)
	}
	if len(ids) != 3 || ids[0] != 2 || ids[1] != 5 || ids[2] != 3 || near.MunicipalityOnlyLeftOut != 1 {
		t.Errorf("near: %v, %+v", ids, near)
	}
	if near.Rows[0].DistanceM != 0 || near.Rows[2].DistanceM == 0 || near.Rows[0].LinkedCases != 0 {
		t.Errorf("near rows: %+v", near.Rows)
	}
	// With a period, an occurrence with no date is counted out rather than lost.
	dated := call("occurrences_near", `{"occurrence_id":1,"radius_m":5000,"from":"2024-01-01","to":"2024-12-31"}`).(OccurrencesNear)
	if dated.RowCount != 2 || dated.UndatedLeftOut != 1 || dated.MunicipalityOnlyLeftOut != 1 {
		t.Errorf("near with a period: %+v", dated)
	}
	if close := call("occurrences_near", `{"latitude":-22.9,"longitude":-43.17,"radius_m":10}`).(OccurrencesNear); close.RowCount != 3 {
		t.Errorf("near a point: %+v", close)
	}

	for _, tc := range [][2]string{
		{"search_persons", `{"name":"a"}`},
		{"person_cases", `{"person_number":999}`},
		{"person_cases", `{}`},
		{"person_cases", `{"name":"Ana Silva"}`}, // never by name: homonyms
		{"case_detail", `{"case_id":"NOPE"}`},
		{"occurrence_detail", `{"occurrence_id":999}`},
		{"inquiry_detail", `{"inquiry_number":"NOPE"}`},
		{"occurrences_near", `{"occurrence_id":1}`},
		{"occurrences_near", `{"radius_m":100}`},
		{"occurrences_near", `{"occurrence_id":4,"radius_m":100}`},    // no place
		{"occurrences_near", `{"occurrence_id":6,"radius_m":100}`},    // a municipality's centre
		{"occurrences_near", `{"occurrence_id":1,"radius_m":100000}`}, // too far
		{"occurrences_near", `{"occurrence_id":1,"latitude":1,"longitude":1,"radius_m":100}`},
		{"occurrences_near", `{"latitude":100,"longitude":1,"radius_m":100}`},
	} {
		if _, err := s.runTool(ctx, asker, tax, toolCall{Name: tc[0], Arguments: tc[1]}); !isToolError(err) {
			t.Errorf("%s %s: got %v", tc[0], tc[1], err)
		}
	}

	// A read that cannot be logged is not served.
	if _, err := s.runTool(ctx, Asker{ID: 999999, Username: "ghost"}, tax, toolCall{Name: "person_cases", Arguments: `{"person_number":1}`}); err == nil || isToolError(err) {
		t.Errorf("an unlogged read: %v", err)
	}
}

// TestFollowUp: what one answer found goes back with the next question, and reaches the model and the record.
func TestFollowUp(t *testing.T) {
	ctx := context.Background()
	sqlDB := testDB(t)
	asker := seed(t, sqlDB)
	seedWalk(t, sqlDB)
	f := newFakeServer(t,
		callTool("c1", "search_persons", `{"name":"Bruno"}`),
		reply("Encontrei Bruno Costa."),
		callTool("c2", "person_cases", `{"person_number":3}`),
		reply("Bruno está vinculado por pista a F-1, número 3."),
	)
	s := NewService(sqlDB, testModel(t, f, Settings{}))
	first, err := s.Ask(ctx, asker, "procure Bruno", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Entities) != 1 || first.Entities[0] != (Entity{Kind: "PERSON", Ref: "3", Label: "Bruno Costa"}) {
		t.Fatalf("entities: %+v", first.Entities)
	}
	second, err := s.Ask(ctx, asker, "quais os casos dele?", []Message{{Role: "user", Text: "procure Bruno"}, {Role: "assistant", Text: first.Answer}}, first.Entities)
	if err != nil {
		t.Fatal(err)
	}
	system := f.requests[2]["messages"].([]any)[0].(map[string]any)["content"].(string)
	if !strings.Contains(system, "PERSON 3: Bruno Costa") {
		t.Errorf("the model was not told what was found: %s", system)
	}
	if len(second.UnverifiedNumbers) != 0 {
		t.Errorf("unverified: %v", second.UnverifiedNumbers)
	}
	var focus string
	if err := sqlDB.QueryRow(`SELECT focus::text FROM ask_queries WHERE id = $1`, second.ID).Scan(&focus); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(focus, "Bruno Costa") {
		t.Errorf("focus record: %s", focus)
	}
}

// TestPersonIdentities: other enrollments matching a person's, with what the evidence amounts to.
func TestPersonIdentities(t *testing.T) {
	ctx := context.Background()
	sqlDB := testDB(t)
	asker := seed(t, sqlDB)
	seedWalk(t, sqlDB)
	for _, statement := range []string{
		`INSERT INTO identity_files (id, register_id, file_type, source_path, storage_ref) VALUES (4, 1, 'nist', 'd', 'd'), (5, 4, 'nist', 'e', 'e')`,
		// Person 1 and person 3 match on face and on fingerprint; person 2 only inconclusively on face.
		`INSERT INTO biometric_decisions (feature_a_id, feature_b_id, modality, role, decision, system_source, confidence, threshold) VALUES
			('1', '3', 'FACIAL', 'SYSTEM', 'POSITIVE', 'test', 0.95, 0.7),
			('4', '5', 'FINGERPRINT', 'SYSTEM', 'POSITIVE', 'test', 300, 40),
			('1', '2', 'FACIAL', 'SYSTEM', 'INCONCLUSIVE', 'test', 0.65, 0.6)`,
	} {
		if _, err := sqlDB.Exec(statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	s := NewService(sqlDB, nil)
	tax, err := s.loadTaxonomy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	identities := func() PersonIdentities {
		t.Helper()
		value, err := s.runTool(ctx, asker, tax, toolCall{Name: "person_identities", Arguments: `{"person_number":1}`})
		if err != nil {
			t.Fatal(err)
		}
		return value.(PersonIdentities)
	}
	got := identities()
	// Person 2 is only inconclusive: counted, not listed.
	if got.RowCount != 1 || got.WeakCandidates != 1 || got.OwnNames != 2 || got.Person.PersonNumber != 1 {
		t.Fatalf("identities: %+v", got)
	}
	bruno := got.Rows[0]
	// The case counts are filled in on both sides: a zero here reads as "no cases" to the model.
	if got.Person.Cases != 2 || got.Person.ReviewedCases != 1 || bruno.Person.Cases != 2 || bruno.Person.ReviewedCases != 0 {
		t.Errorf("case counts: %+v, %+v", got.Person, bruno.Person)
	}
	if bruno.Person.PersonNumber != 3 || bruno.Evidence != evidenceTwoModalities || bruno.SharesName || bruno.SharesBirthDate || bruno.SharesParent || bruno.SharedTraces != 2 {
		t.Errorf("person 3: %+v", bruno)
	}
	if len(bruno.Comparisons) != 2 || bruno.Comparisons[0].Modality != "FACIAL" || *bruno.Comparisons[0].Score != 0.95 ||
		*bruno.Comparisons[1].Threshold != 40 || !strings.Contains(bruno.Comparisons[0].Matches[0], "RG D1 (Ana Silva) = RG D3 (Bruno Costa)") {
		t.Errorf("person 3's comparisons: %+v", bruno.Comparisons)
	}
	// Each person returned is logged as read, one only counted is not, and no parent's name reaches the model.
	for id, want := range map[string]int{"SIM-P-1": 1, "SIM-P-3": 1, "SIM-P-2": 0} {
		if n := accessLogged(t, sqlDB, accessPerson, id); n != want {
			t.Errorf("%s logged %d times", id, n)
		}
	}
	if encoded, _ := json.Marshal(got); strings.Contains(string(encoded), "Mother") {
		t.Errorf("a parent's name reached the model: %s", encoded)
	}

	// An examiner's NEGATIVE outweighs the matcher's POSITIVE, and goes last.
	if _, err := sqlDB.Exec(`INSERT INTO biometric_decisions (feature_a_id, feature_b_id, modality, role, decision, username)
		VALUES ('1', '3', 'FACIAL', 'VERIFICATOR', 'NEGATIVE', 'examiner')`); err != nil {
		t.Fatal(err)
	}
	if rows := identities().Rows; len(rows) != 1 || rows[0].Person.PersonNumber != 3 || rows[0].Evidence != evidenceHumanNegative {
		t.Errorf("after a NEGATIVE: %+v", rows)
	}

	// People confirmed it: the reviewed layer holds both enrollments.
	if _, err := sqlDB.Exec(`INSERT INTO cluster_members (cluster_id, basis, feature_id) VALUES (2, 'REVIEWED', '3')`); err != nil {
		t.Fatal(err)
	}
	if rows := identities().Rows; rows[0].Person.PersonNumber != 3 || rows[0].Evidence != evidenceReviewed {
		t.Errorf("reviewed: %+v", rows)
	}

	// In a case, two people on one trace are flagged as such, not as two people present.
	value, err := s.runTool(ctx, asker, tax, toolCall{Name: "case_detail", Arguments: `{"case_id":"F-1"}`})
	if err != nil {
		t.Fatal(err)
	}
	persons := value.(CaseDetail).Persons
	if len(persons) != 2 || len(persons[0].SameTraceAs) != 1 || persons[0].SameTraceAs[0] != persons[1].PersonNumber {
		t.Errorf("same trace: %+v", persons)
	}

	// Nobody but a weak candidate: an empty list, not an error.
	lonely, err := s.runTool(ctx, asker, tax, toolCall{Name: "person_identities", Arguments: `{"person_number":2}`})
	if err != nil {
		t.Fatal(err)
	}
	if result := lonely.(PersonIdentities); len(result.Rows) != 0 || result.WeakCandidates != 1 {
		t.Errorf("person 2's identities: %+v", result)
	}
	if _, err := s.runTool(ctx, asker, tax, toolCall{Name: "person_identities", Arguments: `{"person_number":999}`}); !isToolError(err) {
		t.Errorf("unknown person: %v", err)
	}
}

// TestIdentityEvidence: what counts as evidence, without a database.
func TestIdentityEvidence(t *testing.T) {
	for want, comparisons := range map[string][]IdentityComparison{
		evidenceReviewed:      {{Reviewed: true, SameCluster: true}},
		evidenceHumanNegative: {{Decision: "POSITIVE", ExaminerDecisions: []string{"VERIFICATOR: NEGATIVE"}}, {Decision: "POSITIVE"}},
		evidenceTwoModalities: {{Decision: "POSITIVE"}, {Decision: "POSITIVE"}},
		evidenceOneModality:   {{Decision: "POSITIVE"}, {Decision: "INCONCLUSIVE"}},
		evidenceIndirect:      {{SameCluster: true}},
		evidenceWeak:          {{Decision: "INCONCLUSIVE"}},
	} {
		if got := identityEvidence(comparisons); got != want {
			t.Errorf("%+v: got %s, want %s", comparisons, got, want)
		}
	}
}

// TestClusterTools: the conflict list and the cluster as a place to navigate.
func TestClusterTools(t *testing.T) {
	ctx := context.Background()
	sqlDB := testDB(t)
	asker := seed(t, sqlDB)
	seedWalk(t, sqlDB)
	// Cluster 1 holds Ana and Bruno, and the matcher compared them on one modality.
	if _, err := sqlDB.Exec(`INSERT INTO biometric_decisions (feature_a_id, feature_b_id, modality, role, decision, system_source, confidence, threshold)
		VALUES ('1', '3', 'FACIAL', 'SYSTEM', 'POSITIVE', 'test', 0.95, 0.7)`); err != nil {
		t.Fatal(err)
	}
	s := NewService(sqlDB, nil)
	tax, err := s.loadTaxonomy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	call := func(tool, arguments string) any {
		t.Helper()
		value, err := s.runTool(ctx, asker, tax, toolCall{Name: tool, Arguments: arguments})
		if err != nil {
			t.Fatalf("%s %s: %v", tool, arguments, err)
		}
		return value
	}
	list := call("identity_conflicts", `{}`).(IdentityConflicts)
	if list.Total != 1 || list.ByKind["ONE_BIOMETRIC_MANY_NAMES"] != 1 || list.ByStrength["ONE_MODALITY"] != 1 || list.Rows[0].ClusterIDs[0] != 1 {
		t.Fatalf("list: %+v", list)
	}
	for _, id := range []string{"SIM-P-1", "SIM-P-3"} {
		if n := accessLogged(t, sqlDB, accessPerson, id); n != 1 {
			t.Errorf("%s logged %d times", id, n)
		}
	}
	if none := call("identity_conflicts", `{"kind":"ONE_IDENTITY_MANY_BIOMETRICS"}`).(IdentityConflicts); none.Total != 0 {
		t.Errorf("filter by kind: %+v", none)
	}
	if none := call("identity_conflicts", `{"modality":"FINGERPRINT"}`).(IdentityConflicts); none.Total != 0 {
		t.Errorf("filter by modality: %+v", none)
	}
	// Entities let the next question say "that cluster".
	refs := map[string]bool{}
	for _, e := range list.entities() {
		refs[e.Kind+":"+e.Ref] = true
	}
	if !refs["CLUSTER:1"] || !refs["PERSON:1"] || !refs["PERSON:3"] {
		t.Errorf("entities: %v", refs)
	}

	detail := call("cluster_detail", `{"cluster_id":1}`).(ClusterResult)
	if detail.Basis != "AUTOMATIC" || detail.Modality != "FACIAL" || len(detail.Persons) != 2 || len(detail.Cases) != 2 || len(detail.Conflicts) != 1 {
		t.Fatalf("cluster: %+v", detail)
	}
	if c := detail.Cases[0]; c.CaseID != "F-1" || c.Occurrence == nil || c.Occurrence.InquiryNumber != "IPL-7" {
		t.Errorf("cluster's case: %+v", c)
	}
	// Reviewed cluster 2 holds Ana alone with the trace, and no conflict.
	if reviewed := call("cluster_detail", `{"cluster_id":2}`).(ClusterResult); reviewed.Basis != "REVIEWED" || len(reviewed.Persons) != 1 || len(reviewed.Conflicts) != 0 {
		t.Errorf("reviewed cluster: %+v", reviewed)
	}

	clusters := call("person_clusters", `{"person_number":1}`).(PersonClusters)
	if clusters.RowCount != 2 || len(clusters.Conflicts) != 1 || clusters.Rows[0].Basis != "AUTOMATIC" {
		t.Errorf("person's clusters: %+v", clusters)
	}
	if n := accessLogged(t, sqlDB, accessPerson, "SIM-P-1"); n != 2 {
		t.Errorf("person read logged %d times", n)
	}
	if other := call("person_clusters", `{"person_number":2}`).(PersonClusters); other.RowCount != 0 || len(other.Conflicts) != 0 {
		t.Errorf("person 2: %+v", other)
	}

	// However a cluster or a conflict is reached, parents' names stay away from the model.
	for name, value := range map[string]any{"list": list, "cluster": detail, "person": clusters} {
		if encoded, _ := json.Marshal(value); strings.Contains(string(encoded), "Mother") || strings.Contains(string(encoded), "Father") {
			t.Errorf("%s: a parent's name reached the model: %s", name, encoded)
		}
	}

	// The walk tools name their clusters, so a case or a person leads to one.
	cases := call("person_cases", `{"person_number":1}`).(PersonCases)
	if len(cases.Rows[0].ClusterIDs) == 0 {
		t.Errorf("person's case has no cluster: %+v", cases.Rows[0])
	}
	if persons := call("case_detail", `{"case_id":"F-1"}`).(CaseDetail).Persons; len(persons[0].ClusterIDs) == 0 {
		t.Errorf("case's person has no cluster: %+v", persons[0])
	}

	for _, tc := range [][2]string{
		{"identity_conflicts", `{"kind":"NOPE"}`},
		{"identity_conflicts", `{"strength":"NOPE"}`},
		{"identity_conflicts", `{"modality":"IRIS"}`},
		{"cluster_detail", `{}`},
		{"cluster_detail", `{"cluster_id":999}`},
		{"person_clusters", `{"person_number":999}`},
	} {
		if _, err := s.runTool(ctx, asker, tax, toolCall{Name: tc[0], Arguments: tc[1]}); !isToolError(err) {
			t.Errorf("%s %s: got %v", tc[0], tc[1], err)
		}
	}
}
