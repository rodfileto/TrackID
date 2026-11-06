package ask

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"

	"github.com/rodfileto/trackid/db/migrations"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	adminURL := os.Getenv("TRACKID_TEST_DATABASE_URL")
	if adminURL == "" {
		t.Skip("TRACKID_TEST_DATABASE_URL not set")
	}
	admin, err := sql.Open("postgres", adminURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })
	name := fmt.Sprintf("trackid_ask_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(`CREATE DATABASE ` + name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Exec(`DROP DATABASE ` + name + ` WITH (FORCE)`) })
	u, err := url.Parse(adminURL)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	sqlDB, err := sql.Open("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	goose.SetBaseFS(migrations.FS)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err := goose.Up(sqlDB, "."); err != nil {
		t.Fatal(err)
	}
	return sqlDB
}

// seed loads a small known scenario and returns an asker.
//
//	1  2024-01-10  PHONE_ROBBERY + ROBBERY_HOMICIDE  DP-001  Centro   COMMERCE  case F-1
//	2  2024-02-20  PHONE_ROBBERY                     DP-001  Centro
//	3  2024-12-31 23:30 local (2025 in UTC)  PHONE_ROBBERY  DP-002  Lapa
//	4  2025-01-01 00:30 local  FRAUD, attempted      DP-002  no place
//	5  no date     PHONE_ROBBERY                     DP-001  Centro
func seed(t *testing.T, sqlDB *sql.DB) Asker {
	t.Helper()
	statements := []string{
		`INSERT INTO municipalities (code, name, uf, source, geom) VALUES ('3304557', 'Niterói', 'RJ', 'test',
			ST_Multi(ST_GeomFromText('POLYGON((-43.8 -23.1,-43.1 -23.1,-43.1 -22.7,-43.8 -22.7,-43.8 -23.1))', 4326))::geography)`,
		`INSERT INTO locations (id, location_id, geom, precision, municipality_code, uf, neighbourhood, place_type) VALUES
			(1, 'L1', ST_SetSRID(ST_MakePoint(-43.17, -22.9), 4326)::geography, 'EXACT', '3304557', 'RJ', 'Centro', 'COMMERCE'),
			(2, 'L2', ST_SetSRID(ST_MakePoint(-43.18, -22.91), 4326)::geography, 'EXACT', '3304557', 'RJ', 'Lapa', 'PUBLIC_ROAD')`,
		`INSERT INTO occurrences (id, agency, occurrence_number, location_id, occurred_from, registered_at, attempted, status, description) VALUES
			(1, 'DP-001', 'A1', 1, '2024-01-10 12:00-03', NOW(), FALSE, 'CLOSED', 'SIMULATED one'),
			(2, 'DP-001', 'A2', 1, '2024-02-20 12:00-03', NOW(), FALSE, 'REGISTERED', 'SIMULATED two'),
			(3, 'DP-002', 'A3', 2, '2024-12-31 23:30-03', NOW(), TRUE, 'REGISTERED', 'SIMULATED three'),
			(4, 'DP-002', 'A4', NULL, '2025-01-01 00:30-03', NOW(), TRUE, 'REGISTERED', 'SIMULATED four'),
			(5, 'DP-001', 'A5', 1, NULL, NOW(), FALSE, 'REGISTERED', 'SIMULATED five')`,
		`SELECT setval('occurrences_id_seq', 5), setval('locations_id_seq', 2)`,
		`INSERT INTO occurrence_classifications (occurrence_id, type_code) VALUES
			(1, 'PHONE_ROBBERY'), (1, 'ROBBERY_HOMICIDE'), (2, 'PHONE_ROBBERY'), (3, 'PHONE_ROBBERY'), (4, 'FRAUD'), (5, 'PHONE_ROBBERY')`,
		`INSERT INTO biometric_cases (case_id, case_type, modality, description, occurrence_id) VALUES
			('F-1', 'CRIMINAL', 'FACIAL', 'SIMULATED case', 1), ('P-1', 'CRIMINAL', 'FINGERPRINT', 'SIMULATED case', NULL)`,
	}
	for _, statement := range statements {
		if _, err := sqlDB.Exec(statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	var id int64
	err := sqlDB.QueryRow(`INSERT INTO users (username, email, password_hash, first_name, last_name, staff_number, job_title)
		VALUES ('analyst', 'analyst@example.org', 'x', 'A', 'B', 'S1', 'J') RETURNING id`).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return Asker{ID: id, Username: "analyst"}
}

func TestTools(t *testing.T) {
	ctx := context.Background()
	sqlDB := testDB(t)
	asker := seed(t, sqlDB)
	s := NewService(sqlDB, nil)
	tax, err := s.loadTaxonomy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	count := func(arguments string) OccurrenceCount {
		t.Helper()
		value, err := s.runTool(ctx, asker, tax, toolCall{Name: "count_occurrences", Arguments: arguments})
		if err != nil {
			t.Fatalf("%s: %v", arguments, err)
		}
		return value.(OccurrenceCount)
	}
	rows := func(c OccurrenceCount) string {
		parts := []string{}
		for _, r := range c.Rows {
			parts = append(parts, fmt.Sprintf("%s=%d", r.Key, r.Count))
		}
		return strings.Join(parts, " ")
	}

	// No filter at all: an empty array must not turn into a filter matching nothing.
	if c := count(`{}`); c.Total != 5 || c.UndatedExcluded != 0 || c.UnplacedExcluded != 0 {
		t.Errorf("everything: %+v", c)
	}
	if c := count(``); c.Total != 5 {
		t.Errorf("no arguments at all: %+v", c)
	}
	// The user's question: type x in a period. Occurrence 3 is 2024 locally, and 5 has no date.
	c := count(`{"types":["PHONE_ROBBERY"],"from":"2024-01-01","to":"2024-12-31"}`)
	if c.Total != 3 || c.UndatedExcluded != 1 {
		t.Errorf("phone robberies in 2024: %+v", c)
	}
	// The period's last day is included whole and the next day's first minutes are not.
	if c := count(`{"from":"2025-01-01","to":"2025-01-01"}`); c.Total != 1 {
		t.Errorf("first day of 2025: %+v", c)
	}
	// A type's lower case is forgiven; a group counts every type in it.
	if c := count(`{"types":["phone_robbery"]}`); c.Total != 4 {
		t.Errorf("lower-case type: %+v", c)
	}
	if c := count(`{"groups":["ROBBERY"]}`); c.Total != 4 {
		t.Errorf("robbery group: %+v", c)
	}
	if c := count(`{"attempted":true}`); c.Total != 2 {
		t.Errorf("attempts: %+v", c)
	}
	// A place filter says what it left out for having no place.
	if c := count(`{"uf":"rj","from":"2025-01-01"}`); c.Total != 0 || c.UnplacedExcluded != 1 {
		t.Errorf("place filter: %+v", c)
	}
	if c := count(`{"neighbourhood":"centro"}`); c.Total != 3 {
		t.Errorf("neighbourhood: %+v", c)
	}

	// Occurrence 1 has two types: it counts under each, once in the total.
	if c := count(`{"group_by":"type"}`); c.Total != 5 || rows(c) != "PHONE_ROBBERY=4 FRAUD=1 ROBBERY_HOMICIDE=1" {
		t.Errorf("by type: %d, %s", c.Total, rows(c))
	}
	if c := count(`{"group_by":"group"}`); rows(c) != "ROBBERY=4 FRAUD=1 LETHAL=1" {
		t.Errorf("by group: %s", rows(c))
	}
	// Months in time order, local time, with undated occurrences under no month.
	if c := count(`{"group_by":"month","from":"2024-01-01"}`); rows(c) != "2024-01=1 2024-02=1 2024-12=1 2025-01=1" {
		t.Errorf("by month: %s", rows(c))
	}
	if c := count(`{"group_by":"agency"}`); rows(c) != "DP-001=3 DP-002=2" || c.Rows[0].SharePercent != 60 {
		t.Errorf("by agency: %+v", c)
	}
	if c := count(`{"group_by":"municipality"}`); rows(c) != "Niterói/RJ=4 =1" {
		t.Errorf("by municipality: %s", rows(c))
	}

	// The model's mistakes come back as mistakes, without running anything.
	for _, arguments := range []string{
		`{"types":["ROUBO"]}`, `{"groups":["NOPE"]}`, `{"from":"01/02/2024"}`, `{"from":"2025-01-01","to":"2024-01-01"}`,
		`{"group_by":"person"}`, `{"filter":"x"}`, `{"uf":"Rio"}`, `{"municipality_code":"123"}`, `not json`,
		`{"types":["x'); DROP TABLE occurrences;--"]}`,
	} {
		if _, err := s.runTool(ctx, asker, tax, toolCall{Name: "count_occurrences", Arguments: arguments}); !isToolError(err) {
			t.Errorf("%s: got %v", arguments, err)
		}
	}
	if _, err := s.runTool(ctx, asker, tax, toolCall{Name: "drop_everything"}); !isToolError(err) {
		t.Errorf("unknown tool: %v", err)
	}

	listed, err := s.runTool(ctx, asker, tax, toolCall{Name: "list_occurrences", Arguments: `{"types":["ROBBERY_HOMICIDE"]}`})
	if err != nil {
		t.Fatal(err)
	}
	list := listed.(OccurrenceList)
	if list.RowCount != 1 || list.Rows[0].Number != "A1" || list.Rows[0].OccurredOn != "2024-01-10" || string(list.Rows[0].Cases) != `["F-1"]` {
		t.Errorf("list: %+v", list)
	}

	cases := func(arguments string) CaseCount {
		t.Helper()
		value, err := s.runTool(ctx, asker, tax, toolCall{Name: "count_cases", Arguments: arguments})
		if err != nil {
			t.Fatalf("%s: %v", arguments, err)
		}
		return value.(CaseCount)
	}
	if c := cases(`{}`); c.Total != 2 {
		t.Errorf("cases: %+v", c)
	}
	if c := cases(`{"group_by":"linked"}`); c.Total != 2 || c.RowCount != 2 {
		t.Errorf("cases by link: %+v", c)
	}
	// A case with two occurrence types counts once in the total, twice in the rows.
	if c := cases(`{"types":["PHONE_ROBBERY","ROBBERY_HOMICIDE"],"group_by":"type"}`); c.Total != 1 || c.RowCount != 2 {
		t.Errorf("cases by type: %+v", c)
	}

	// Accents and case don't matter, and a pattern character is only a character.
	found, err := s.runTool(ctx, asker, tax, toolCall{Name: "find_municipality", Arguments: `{"name":"niteroi"}`})
	if err != nil {
		t.Fatal(err)
	}
	if encoded, _ := json.Marshal(found); !strings.Contains(string(encoded), "3304557") {
		t.Errorf("find_municipality: %s", encoded)
	}
	none, err := s.runTool(ctx, asker, tax, toolCall{Name: "find_municipality", Arguments: `{"name":"%"}`})
	if err != nil {
		t.Fatal(err)
	}
	if encoded, _ := json.Marshal(none); !strings.Contains(string(encoded), `"rowCount":0`) {
		t.Errorf("a wildcard matched: %s", encoded)
	}
	value, err := s.runTool(ctx, asker, tax, toolCall{Name: "data_coverage"})
	if err != nil {
		t.Fatal(err)
	}
	if encoded, _ := json.Marshal(value); !strings.Contains(string(encoded), `"first_occurred":"2024-01-10"`) {
		t.Errorf("coverage: %s", encoded)
	}
}

// TestAsk: a question through the whole loop against a scripted server, with its record.
func TestAsk(t *testing.T) {
	ctx := context.Background()
	sqlDB := testDB(t)
	asker := seed(t, sqlDB)
	f := newFakeServer(t,
		callTool("c1", "count_occurrences", `{"types":["WRONG"]}`),
		callTool("c2", "count_occurrences", `{"types":["PHONE_ROBBERY"],"from":"2024-01-01","to":"2024-12-31"}`),
		reply("Foram 3 roubos de celular em 2024, e 77 outros."),
	)
	s := NewService(sqlDB, testModel(t, f, Settings{}))

	answer, err := s.Ask(ctx, asker, "Quantos roubos de celular em 2024?", []Message{{Role: "user", Text: "oi"}, {Role: "assistant", Text: "olá"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(answer.Answer, "Foram 3") || answer.ID == 0 || answer.Usage.OutputTokens != 15 {
		t.Errorf("answer: %+v", answer)
	}
	// The invented figure is flagged, and the real ones are not.
	if len(answer.UnverifiedNumbers) != 1 || answer.UnverifiedNumbers[0] != "77" {
		t.Errorf("unverified: %v", answer.UnverifiedNumbers)
	}
	// The wrong type went back to the model as an error it could correct.
	if len(answer.Steps) != 2 || !strings.Contains(answer.Steps[0].Error, "PHONE_ROBBERY") || answer.Steps[1].Error != "" {
		t.Errorf("steps: %+v", answer.Steps)
	}
	second := f.requests[1]["messages"].([]any)
	if last := second[len(second)-1].(map[string]any); !strings.HasPrefix(last["content"].(string), "ERROR: types: unknown value") {
		t.Errorf("the model was told %v", last)
	}
	// The system prompt carries the date and the taxonomy.
	system := f.requests[0]["messages"].([]any)[0].(map[string]any)["content"].(string)
	if !strings.Contains(system, "POST_WITHDRAWAL_ROBBERY: roubo após saque") || !strings.Contains(system, "Today is ") ||
		!strings.Contains(system, "never write it in an answer") {
		t.Errorf("system prompt: %s", system)
	}

	var username, question, stored, endpoint string
	var external bool
	var steps []Step
	var rawSteps []byte
	err = sqlDB.QueryRow(`SELECT username, question, answer, endpoint, external, steps FROM ask_queries WHERE id = $1`, answer.ID).
		Scan(&username, &question, &stored, &endpoint, &external, &rawSteps)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(rawSteps, &steps); err != nil {
		t.Fatal(err)
	}
	if username != "analyst" || question == "" || stored != answer.Answer || external || len(steps) != 2 || !strings.HasPrefix(endpoint, "127.0.0.1:") {
		t.Errorf("record: %s %q %q %s %t %+v", username, question, stored, endpoint, external, steps)
	}

	// A failure is recorded too, with the steps it got through.
	failing := newFakeServer(t, callTool("c1", "data_coverage", ``), `{"error":{"message":"overloaded"}}`)
	if _, err := NewService(sqlDB, testModel(t, failing, Settings{})).Ask(ctx, asker, "falha?", nil, nil); err == nil {
		t.Fatal("no error")
	}
	var failure string
	if err := sqlDB.QueryRow(`SELECT error, steps FROM ask_queries WHERE question = 'falha?'`).Scan(&failure, &rawSteps); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(failure, "overloaded") || !strings.Contains(string(rawSteps), "data_coverage") {
		t.Errorf("failure record: %s, %s", failure, rawSteps)
	}

	// A model that never stops calling tools stops.
	loops := make([]string, maxRounds)
	for i := range loops {
		loops[i] = callTool("c", "data_coverage", `{}`)
	}
	if _, err := NewService(sqlDB, testModel(t, newFakeServer(t, loops...), Settings{})).Ask(ctx, asker, "loop?", nil, nil); err == nil || !strings.Contains(err.Error(), "rounds") {
		t.Errorf("loop: %v", err)
	}
}

// TestExternalModelOnlyReadsSyntheticData: a server outside the institute gets synthetic data or
// nothing, and the model is never called once anything else is in the database.
func TestExternalModelOnlyReadsSyntheticData(t *testing.T) {
	ctx := context.Background()
	sqlDB := testDB(t)
	asker := seed(t, sqlDB)
	f := newFakeServer(t, reply("Nada."))
	// The fake listens on loopback, which is local: look at it through a public-looking host.
	model := testModel(t, f, Settings{})
	if model.External() {
		t.Fatal("loopback counted as external")
	}
	external, err := NewModel(Settings{Endpoint: "https://model.invalid/v1", Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	external.http.Transport = rewriteHost{to: f.URL}

	if _, err := NewService(sqlDB, external).Ask(ctx, asker, "sintéticos?", nil, nil); err != nil {
		t.Fatalf("synthetic data refused: %v", err)
	}
	if len(f.requests) != 1 {
		t.Fatalf("%d requests", len(f.requests))
	}

	for _, statement := range []string{
		`INSERT INTO occurrences (agency, occurrence_number, registered_at, status, description) VALUES ('DP', 'REAL', NOW(), 'REGISTERED', 'A real report')`,
		`UPDATE occurrences SET description = 'SIMULATED again' WHERE occurrence_number = 'REAL';
		 INSERT INTO biometric_cases (case_id, case_type, modality, description) VALUES ('REAL', 'CRIMINAL', 'FACIAL', 'A real case')`,
		`UPDATE biometric_cases SET description = 'SIMULATED again' WHERE case_id = 'REAL';
		 INSERT INTO persons (person_id) VALUES ('A-REAL-PERSON')`,
	} {
		if _, err := sqlDB.Exec(statement); err != nil {
			t.Fatal(err)
		}
		if _, err := NewService(sqlDB, external).Ask(ctx, asker, "reais?", nil, nil); !errors.Is(err, ErrRealData) {
			t.Errorf("after %q: %v", statement, err)
		}
		if len(f.requests) != 1 {
			t.Fatalf("the model was called: %d requests", len(f.requests))
		}
	}
	var external2 bool
	var failure string
	if err := sqlDB.QueryRow(`SELECT external, error FROM ask_queries WHERE question = 'reais?' LIMIT 1`).Scan(&external2, &failure); err != nil {
		t.Fatal(err)
	}
	if !external2 || failure == "" {
		t.Errorf("refusal record: %t %q", external2, failure)
	}

	// A local model is not limited: the guard is about where the data goes.
	local := newFakeServer(t, reply("Ok."))
	if _, err := NewService(sqlDB, testModel(t, local, Settings{})).Ask(ctx, asker, "local?", nil, nil); err != nil {
		t.Errorf("local model: %v", err)
	}
}

// rewriteHost sends a request for another host to a test server.
type rewriteHost struct{ to string }

func (r rewriteHost) RoundTrip(request *http.Request) (*http.Response, error) {
	target, err := url.Parse(r.to)
	if err != nil {
		return nil, err
	}
	clone := request.Clone(request.Context())
	clone.URL.Scheme, clone.URL.Host = target.Scheme, target.Host
	return http.DefaultTransport.RoundTrip(clone)
}
