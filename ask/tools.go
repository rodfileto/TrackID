package ask

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/rodfileto/trackid/db"
)

const (
	// maxBreakdownRows and maxListRows bound what one tool call returns to the model.
	maxBreakdownRows = 200
	maxListRows      = 50
	// maxDescription bounds an occurrence's description in a list.
	maxDescription = 300
)

// reportZone is the time zone a period is cut in, as on the occurrence map.
var reportZone = func() *time.Location {
	if z, err := time.LoadLocation("America/Sao_Paulo"); err == nil {
		return z
	}
	return time.FixedZone("BRT", -3*3600)
}()

var (
	occurrenceDimensions = []string{"none", "type", "group", "month", "year", "status", "agency", "uf", "municipality", "neighbourhood", "place_type"}
	caseDimensions       = []string{"none", "modality", "case_type", "linked", "type", "group"}
	statuses             = []string{"REGISTERED", "UNDER_INVESTIGATION", "INQUIRY_OPENED", "CLOSED"}
	placeTypes           = []string{"PUBLIC_ROAD", "RESIDENCE", "COMMERCE", "BANK", "GOVERNMENT_OFFICE", "TRANSPORT", "RURAL", "OTHER"}
	modalities           = []string{"FACIAL", "FINGERPRINT"}
	caseTypes            = []string{"CRIMINAL", "CIVIL"}
	municipalityCode     = regexp.MustCompile(`^[0-9]{7}$`)
	stateCode            = regexp.MustCompile(`^[A-Z]{2}$`)
)

// toolError is a mistake in a tool call, which the model is told so it can correct itself.
type toolError struct{ message string }

func (e toolError) Error() string { return e.message }

func badCall(format string, args ...any) error { return toolError{fmt.Sprintf(format, args...)} }

func stringList(description string) map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": description}
}

func text(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func enum(description string, values []string) map[string]any {
	return map[string]any{"type": "string", "description": description, "enum": values}
}

func period() (from, to map[string]any) {
	return text("First day of the period, YYYY-MM-DD, inclusive. A period counts by when the occurrence began."),
		text("Last day of the period, YYYY-MM-DD, inclusive.")
}

// occurrenceSchema is the filters count_occurrences and list_occurrences share.
func occurrenceSchema() map[string]any {
	from, to := period()
	return map[string]any{
		"types":             stringList("Occurrence type codes; an occurrence matches if it has any of them. Prefer groups for a general word such as 'roubo'."),
		"groups":            stringList("Occurrence groups (LETHAL, PERSON, ROBBERY, THEFT, FRAUD, DRUGS, CIVIL, OTHER); an occurrence matches if any of its types is in one."),
		"from":              from,
		"to":                to,
		"statuses":          map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": statuses}},
		"attempted":         map[string]any{"type": "boolean", "description": "True keeps only attempts (tentativas), false only consummated crimes."},
		"agency":            text("The police agency, exactly as recorded, e.g. DP-005."),
		"uf":                text("The state, two letters, e.g. RJ."),
		"municipality_code": text("The IBGE municipality code, seven digits; find_municipality gives it."),
		"neighbourhood":     text("The neighbourhood's name, exactly as recorded, ignoring case. Group by neighbourhood to see the names."),
		"place_types":       map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": placeTypes}},
	}
}

func catalog() []Tool {
	count := occurrenceSchema()
	count["group_by"] = enum("How to break the count down. Months and years with no occurrences are not listed.", occurrenceDimensions)
	caseFrom, caseTo := period()
	return append([]Tool{
		{
			Name: "data_coverage",
			Description: "What the database holds: how many occurrences and cases, the first and last occurrence dates, and how many " +
				"occurrences have no date or no place. Use it to learn the limits of the data, or when a question has no period and that matters.",
			Schema: map[string]any{},
		},
		{
			Name: "count_occurrences",
			Description: "Counts registered occurrences matching the filters, optionally broken down. Returns the total of distinct " +
				"occurrences, rows with each value's count and share of the total, and how many occurrences a period or place filter " +
				"left out only because they have no date or no place. An occurrence with several types counts once under each " +
				"type or group, so those rows can add up to more than the total. To compare periods, call it once per period.",
			Schema: count,
		},
		{
			Name: "list_occurrences",
			Description: fmt.Sprintf("Lists the individual occurrences matching the filters, newest first, at most %d: identifiers, date, "+
				"types, status, place, description and the linked cases. Use it when asked which occurrences, not how many.", maxListRows),
			Schema: occurrenceSchema(),
		},
		{
			Name: "count_cases",
			Description: "Counts biometric cases (the forensic cases opened from occurrences), optionally broken down. The type, group and " +
				"period filters read the case's linked occurrence, so they leave out cases with no occurrence.",
			Schema: map[string]any{
				"modality":  enum("The biometric the case works with.", modalities),
				"case_type": enum("The nature of the case.", caseTypes),
				"types":     stringList("The linked occurrence's type codes."),
				"groups":    stringList("The linked occurrence's groups."),
				"from":      caseFrom,
				"to":        caseTo,
				"group_by":  enum("How to break the count down; linked separates cases with and without an occurrence.", caseDimensions),
			},
		},
		{
			Name: "find_municipality",
			Description: "Finds municipalities by name, ignoring case and accents, with their IBGE code and how many occurrences each holds. " +
				"It finds nothing when the deployment has loaded no municipality names (data_coverage says how many are loaded); group " +
				"occurrences by municipality to see the codes in use.",
			Schema: map[string]any{"name": text("The name or part of it.")},
		},
	}, walkCatalog()...)
}

// occurrenceFilters are a tool call's filters as the model wrote them.
type occurrenceFilters struct {
	Types            []string `json:"types"`
	Groups           []string `json:"groups"`
	From             string   `json:"from"`
	To               string   `json:"to"`
	Statuses         []string `json:"statuses"`
	Attempted        *bool    `json:"attempted"`
	Agency           string   `json:"agency"`
	UF               string   `json:"uf"`
	MunicipalityCode string   `json:"municipality_code"`
	Neighbourhood    string   `json:"neighbourhood"`
	PlaceTypes       []string `json:"place_types"`
}

// resolvedFilters are the filters checked and ready for a query. The slices are never nil: a NULL
// array would turn a filter's "cardinality = 0" into NULL and match nothing.
type resolvedFilters struct {
	types, groups, statuses, placeTypes []string
	after, before                       sql.NullTime
	attempted                           sql.NullBool
	agency, uf, municipality            sql.NullString
	neighbourhood                       sql.NullString
}

// taxonomy is the occurrence types and groups the database knows.
type taxonomy struct {
	types  []string
	groups []string
}

func (s *Service) loadTaxonomy(ctx context.Context) (taxonomy, error) {
	rows, err := db.New(s.db).ListOccurrenceTypes(ctx)
	if err != nil {
		return taxonomy{}, err
	}
	var tax taxonomy
	for _, r := range rows {
		tax.types = append(tax.types, r.Code)
		if !slices.Contains(tax.groups, r.Grp) {
			tax.groups = append(tax.groups, r.Grp)
		}
	}
	slices.Sort(tax.groups)
	return tax, nil
}

func checkAll(name string, got, allowed []string) ([]string, error) {
	out := make([]string, 0, len(got))
	for _, v := range got {
		v = strings.ToUpper(strings.TrimSpace(v))
		if !slices.Contains(allowed, v) {
			return nil, badCall("%s: unknown value %q; allowed: %s", name, v, strings.Join(allowed, ", "))
		}
		out = append(out, v)
	}
	return out, nil
}

func optional(s string) sql.NullString {
	s = strings.TrimSpace(s)
	return sql.NullString{String: s, Valid: s != ""}
}

// parsePeriod turns inclusive dates into the half-open range the queries take, in reportZone.
func parsePeriod(from, to string) (after, before sql.NullTime, err error) {
	var first, last time.Time
	if from != "" {
		if first, err = time.ParseInLocation("2006-01-02", from, reportZone); err != nil {
			return after, before, badCall("from: %q is not a date in YYYY-MM-DD", from)
		}
		after = sql.NullTime{Time: first, Valid: true}
	}
	if to != "" {
		if last, err = time.ParseInLocation("2006-01-02", to, reportZone); err != nil {
			return after, before, badCall("to: %q is not a date in YYYY-MM-DD", to)
		}
		before = sql.NullTime{Time: last.AddDate(0, 0, 1), Valid: true}
	}
	if after.Valid && before.Valid && last.Before(first) {
		return after, before, badCall("the period ends (%s) before it begins (%s)", to, from)
	}
	return after, before, nil
}

func (f occurrenceFilters) resolve(tax taxonomy) (resolvedFilters, error) {
	var r resolvedFilters
	var err error
	if r.types, err = checkAll("types", f.Types, tax.types); err != nil {
		return r, err
	}
	if r.groups, err = checkAll("groups", f.Groups, tax.groups); err != nil {
		return r, err
	}
	if r.statuses, err = checkAll("statuses", f.Statuses, statuses); err != nil {
		return r, err
	}
	if r.placeTypes, err = checkAll("place_types", f.PlaceTypes, placeTypes); err != nil {
		return r, err
	}
	if r.after, r.before, err = parsePeriod(f.From, f.To); err != nil {
		return r, err
	}
	if f.Attempted != nil {
		r.attempted = sql.NullBool{Bool: *f.Attempted, Valid: true}
	}
	r.agency = optional(f.Agency)
	r.neighbourhood = optional(f.Neighbourhood)
	r.uf = optional(strings.ToUpper(f.UF))
	if r.uf.Valid && !stateCode.MatchString(r.uf.String) {
		return r, badCall("uf: %q is not a two-letter state", f.UF)
	}
	r.municipality = optional(f.MunicipalityCode)
	if r.municipality.Valid && !municipalityCode.MatchString(r.municipality.String) {
		return r, badCall("municipality_code: %q is not a seven-digit IBGE code; find_municipality gives it", f.MunicipalityCode)
	}
	return r, nil
}

// decodeArguments reads a tool call's arguments, refusing a field the tool doesn't have: a filter
// silently ignored would give a wrong count that looks right.
func decodeArguments(arguments string, into any) error {
	if strings.TrimSpace(arguments) == "" {
		arguments = "{}"
	}
	decoder := json.NewDecoder(bytes.NewReader([]byte(arguments)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil {
		return badCall("invalid arguments: %v", err)
	}
	return nil
}

// runTool runs one tool call. A toolError is a mistake to tell the model about; any other error is
// the system's.
func (s *Service) runTool(ctx context.Context, asker Asker, tax taxonomy, call toolCall) (any, error) {
	switch call.Name {
	case "search_persons":
		return s.searchPersons(ctx, call.Arguments)
	case "person_cases":
		return s.personCases(ctx, asker, call.Arguments)
	case "identity_conflicts":
		return s.identityConflicts(ctx, asker, call.Arguments)
	case "cluster_detail":
		return s.clusterDetail(ctx, call.Arguments)
	case "person_clusters":
		return s.personClusters(ctx, asker, call.Arguments)
	case "person_identities":
		return s.personIdentities(ctx, asker, call.Arguments)
	case "case_detail":
		return s.caseDetail(ctx, asker, call.Arguments)
	case "find_occurrence":
		return s.findOccurrence(ctx, call.Arguments)
	case "occurrence_detail":
		return s.occurrenceDetail(ctx, asker, call.Arguments)
	case "inquiry_detail":
		return s.inquiryDetail(ctx, asker, call.Arguments)
	case "occurrences_near":
		return s.occurrencesNear(ctx, tax, call.Arguments)
	case "data_coverage":
		return db.New(s.db).AskDataCoverage(ctx)
	case "count_occurrences":
		return s.countOccurrences(ctx, tax, call.Arguments)
	case "list_occurrences":
		return s.listOccurrences(ctx, tax, call.Arguments)
	case "count_cases":
		return s.countCases(ctx, tax, call.Arguments)
	case "find_municipality":
		return s.findMunicipality(ctx, call.Arguments)
	}
	return nil, badCall("there is no tool %q", call.Name)
}

// BreakdownRow is one value of a dimension. Key is empty when occurrences have no such value.
type BreakdownRow struct {
	Key          string  `json:"key"`
	Count        int64   `json:"count"`
	SharePercent float64 `json:"sharePercent"`
}

// OccurrenceCount is count_occurrences' result.
type OccurrenceCount struct {
	Total            int64          `json:"total"`
	UndatedExcluded  int64          `json:"undatedExcluded"`
	UnplacedExcluded int64          `json:"unplacedExcluded"`
	GroupBy          string         `json:"groupBy"`
	RowCount         int            `json:"rowCount"`
	Rows             []BreakdownRow `json:"rows"`
	Truncated        bool           `json:"truncated"`
}

func (s *Service) countOccurrences(ctx context.Context, tax taxonomy, arguments string) (any, error) {
	var args struct {
		occurrenceFilters
		GroupBy string `json:"group_by"`
	}
	if err := decodeArguments(arguments, &args); err != nil {
		return nil, err
	}
	if args.GroupBy == "" {
		args.GroupBy = "none"
	}
	if !slices.Contains(occurrenceDimensions, args.GroupBy) {
		return nil, badCall("group_by: unknown value %q; allowed: %s", args.GroupBy, strings.Join(occurrenceDimensions, ", "))
	}
	f, err := args.resolve(tax)
	if err != nil {
		return nil, err
	}
	q := db.New(s.db)
	summary, err := q.AskOccurrenceSummary(ctx, db.AskOccurrenceSummaryParams{
		Types: f.types, Groups: f.groups, Statuses: f.statuses, PlaceTypes: f.placeTypes,
		OccurredAfter: f.after, OccurredBefore: f.before, Attempted: f.attempted, Agency: f.agency,
		Uf: f.uf, MunicipalityCode: f.municipality, Neighbourhood: f.neighbourhood,
	})
	if err != nil {
		return nil, err
	}
	result := OccurrenceCount{
		Total: summary.Matched, UndatedExcluded: summary.UndatedExcluded, UnplacedExcluded: summary.UnplacedExcluded,
		GroupBy: args.GroupBy, Rows: []BreakdownRow{},
	}
	if args.GroupBy == "none" {
		return result, nil
	}
	rows, err := q.AskOccurrenceBreakdown(ctx, db.AskOccurrenceBreakdownParams{
		Dimension: args.GroupBy, MaxRows: maxBreakdownRows + 1,
		Types: f.types, Groups: f.groups, Statuses: f.statuses, PlaceTypes: f.placeTypes,
		OccurredAfter: f.after, OccurredBefore: f.before, Attempted: f.attempted, Agency: f.agency,
		Uf: f.uf, MunicipalityCode: f.municipality, Neighbourhood: f.neighbourhood,
	})
	if err != nil {
		return nil, err
	}
	if len(rows) > maxBreakdownRows {
		rows, result.Truncated = rows[:maxBreakdownRows], true
	}
	for _, r := range rows {
		result.Rows = append(result.Rows, BreakdownRow{Key: r.Key, Count: r.Occurrences, SharePercent: share(r.Occurrences, summary.Matched)})
	}
	result.RowCount = len(result.Rows)
	return result, nil
}

func share(n, total int64) float64 {
	if total == 0 {
		return 0
	}
	return math.Round(float64(n)/float64(total)*1000) / 10
}

// ListedOccurrence is one occurrence in list_occurrences' result. ID is what occurrence_detail takes;
// the numbers are what an analyst knows it by.
type ListedOccurrence struct {
	ID            int64           `json:"id"`
	TrackIDNumber string          `json:"trackidNumber,omitempty"`
	Agency        string          `json:"agency,omitempty"`
	Number        string          `json:"number,omitempty"`
	InquiryNumber string          `json:"inquiryNumber,omitempty"`
	OccurredOn    string          `json:"occurredOn,omitempty"`
	Attempted     *bool           `json:"attempted,omitempty"`
	Status        string          `json:"status"`
	Types         json.RawMessage `json:"types"`
	PlaceName     string          `json:"placeName,omitempty"`
	Neighbourhood string          `json:"neighbourhood,omitempty"`
	UF            string          `json:"uf,omitempty"`
	PlaceType     string          `json:"placeType,omitempty"`
	Description   string          `json:"description"`
	Cases         json.RawMessage `json:"cases"`
}

// OccurrenceList is list_occurrences' result.
type OccurrenceList struct {
	RowCount  int                `json:"rowCount"`
	Rows      []ListedOccurrence `json:"rows"`
	Truncated bool               `json:"truncated"`
}

func (s *Service) listOccurrences(ctx context.Context, tax taxonomy, arguments string) (any, error) {
	var args occurrenceFilters
	if err := decodeArguments(arguments, &args); err != nil {
		return nil, err
	}
	f, err := args.resolve(tax)
	if err != nil {
		return nil, err
	}
	rows, err := db.New(s.db).AskListOccurrences(ctx, db.AskListOccurrencesParams{
		MaxRows: maxListRows + 1,
		Types:   f.types, Groups: f.groups, Statuses: f.statuses, PlaceTypes: f.placeTypes,
		OccurredAfter: f.after, OccurredBefore: f.before, Attempted: f.attempted, Agency: f.agency,
		Uf: f.uf, MunicipalityCode: f.municipality, Neighbourhood: f.neighbourhood,
	})
	if err != nil {
		return nil, err
	}
	result := OccurrenceList{Rows: []ListedOccurrence{}}
	if len(rows) > maxListRows {
		rows, result.Truncated = rows[:maxListRows], true
	}
	for _, r := range rows {
		item := ListedOccurrence{
			ID: r.ID, Agency: r.Agency.String, Number: r.OccurrenceNumber.String, InquiryNumber: r.InquiryNumber.String,
			Status: r.Status, Types: r.Types, PlaceName: r.PlaceName.String, Neighbourhood: r.Neighbourhood.String,
			UF: r.Uf.String, PlaceType: r.PlaceType.String, Description: truncate(r.Description, maxDescription), Cases: r.Cases,
		}
		if r.OccurrenceYear.Valid && r.OccurrenceSeq.Valid {
			item.TrackIDNumber = fmt.Sprintf("%d.%04d", r.OccurrenceYear.Int32, r.OccurrenceSeq.Int32)
		}
		if r.OccurredFrom.Valid {
			item.OccurredOn = r.OccurredFrom.Time.In(reportZone).Format("2006-01-02")
		}
		if r.Attempted.Valid {
			item.Attempted = &r.Attempted.Bool
		}
		result.Rows = append(result.Rows, item)
	}
	result.RowCount = len(result.Rows)
	return result, nil
}

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}

// CaseCount is count_cases' result. A case counts once in Total, and by type or group once under
// each of its occurrence's.
type CaseCount struct {
	Total    int64          `json:"total"`
	GroupBy  string         `json:"groupBy"`
	RowCount int            `json:"rowCount"`
	Rows     []BreakdownRow `json:"rows"`
}

func (s *Service) countCases(ctx context.Context, tax taxonomy, arguments string) (any, error) {
	var args struct {
		Modality string   `json:"modality"`
		CaseType string   `json:"case_type"`
		Types    []string `json:"types"`
		Groups   []string `json:"groups"`
		From     string   `json:"from"`
		To       string   `json:"to"`
		GroupBy  string   `json:"group_by"`
	}
	if err := decodeArguments(arguments, &args); err != nil {
		return nil, err
	}
	if args.GroupBy == "" {
		args.GroupBy = "none"
	}
	if !slices.Contains(caseDimensions, args.GroupBy) {
		return nil, badCall("group_by: unknown value %q; allowed: %s", args.GroupBy, strings.Join(caseDimensions, ", "))
	}
	types, err := checkAll("types", args.Types, tax.types)
	if err != nil {
		return nil, err
	}
	groups, err := checkAll("groups", args.Groups, tax.groups)
	if err != nil {
		return nil, err
	}
	if args.Modality != "" && !slices.Contains(modalities, args.Modality) {
		return nil, badCall("modality: unknown value %q; allowed: %s", args.Modality, strings.Join(modalities, ", "))
	}
	if args.CaseType != "" && !slices.Contains(caseTypes, args.CaseType) {
		return nil, badCall("case_type: unknown value %q; allowed: %s", args.CaseType, strings.Join(caseTypes, ", "))
	}
	after, before, err := parsePeriod(args.From, args.To)
	if err != nil {
		return nil, err
	}
	params := db.AskCountCasesParams{
		MaxRows: maxBreakdownRows, Modality: optional(args.Modality), CaseType: optional(args.CaseType),
		Types: types, Groups: groups, OccurredAfter: after, OccurredBefore: before,
	}
	q := db.New(s.db)
	// The total counts each case once, whatever the breakdown does.
	params.Dimension = "none"
	totals, err := q.AskCountCases(ctx, params)
	if err != nil {
		return nil, err
	}
	result := CaseCount{GroupBy: args.GroupBy, Rows: []BreakdownRow{}}
	if len(totals) > 0 {
		result.Total = totals[0].Cases
	}
	if args.GroupBy == "none" {
		return result, nil
	}
	params.Dimension = args.GroupBy
	rows, err := q.AskCountCases(ctx, params)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		result.Rows = append(result.Rows, BreakdownRow{Key: r.Key, Count: r.Cases, SharePercent: share(r.Cases, result.Total)})
	}
	result.RowCount = len(result.Rows)
	return result, nil
}

func (s *Service) findMunicipality(ctx context.Context, arguments string) (any, error) {
	var args struct {
		Name string `json:"name"`
	}
	if err := decodeArguments(arguments, &args); err != nil {
		return nil, err
	}
	if strings.TrimSpace(args.Name) == "" {
		return nil, badCall("name is required")
	}
	rows, err := db.New(s.db).AskFindMunicipalities(ctx, strings.TrimSpace(args.Name))
	if err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []db.AskFindMunicipalitiesRow{}
	}
	return map[string]any{"rowCount": len(rows), "rows": rows}, nil
}

// isToolError says whether an error is a mistake in the call rather than the system's failure.
func isToolError(err error) bool {
	var te toolError
	return errors.As(err, &te)
}
