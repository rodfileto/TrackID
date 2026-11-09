package api

import (
	"database/sql"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/rodfileto/trackid/cases"
	"github.com/rodfileto/trackid/distribution"
)

// caseYearPattern matches a single case_id segment (case_ids are dot-separated,
// e.g. "01.2026.01.SRMG.00270" or "2026.0001") that looks like a year, whichever
// position it appears in.
var caseYearPattern = regexp.MustCompile(`^(19|20)\d{2}$`)

// assignedToFilter keeps the biometric_cases where the user (placeholder p, e.g. "$3") currently
// holds an analysis or collected material at a scene, what anyone but an analyst or coordinator may list
// (distribution.SeesEveryCase), except while they work
// one of its analyses blind (distribution.BlindIn; the step whose stage it is, other than the
// first examiner's, as analyses.BlindWork).
func assignedToFilter(p string) string {
	return "(id IN (SELECT ca.biometric_case_id FROM case_analyses ca" +
		" JOIN current_analysis_assignments caa ON caa.case_analysis_id = ca.id WHERE caa.user_id = " + p + ")" +
		" OR id IN (SELECT cc.biometric_case_id FROM case_collections cc WHERE cc.collected_by = " + p + "))" +
		" AND id NOT IN (SELECT ca.biometric_case_id FROM case_analyses ca" +
		" JOIN current_analysis_assignments caa ON caa.case_analysis_id = ca.id WHERE caa.user_id = " + p +
		" AND ca.status = 'OPEN' AND caa.step = CASE ca.stage WHEN 'VERIFICATION' THEN 'REVIEWER'" +
		" WHEN 'INCONSISTENCE' THEN 'INCONSISTENCE' END)"
}

// unitStates gives every unit its state: its own, or else its parent's, since a unit with none is where its parent
// is. A root with none takes its organization's, which only a state's organization has (018_organizations.sql). A
// unit may still have no state, as a federal organization's root: its cases are under none.
const unitStates = `WITH RECURSIVE unit_states AS (
	SELECT u.id, COALESCE(u.uf, o.uf)::text AS uf FROM units u JOIN organizations o ON o.id = u.organization_id WHERE u.parent_id IS NULL
	UNION ALL
	SELECT child.id, COALESCE(child.uf::text, parent.uf) FROM units child JOIN unit_states parent ON child.parent_id = parent.id
)`

type ListResponse struct {
	Items      []cases.Case `json:"items"`
	Total      int          `json:"total"`
	Page       int          `json:"page"`
	PageSize   int          `json:"pageSize"`
	TotalPages int          `json:"totalPages"`
}

func ListCasesHandler(db *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		if db == nil {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
			return
		}
		page := parsePositiveInt(context.Query("page"), 1)
		pageSize := parsePositiveInt(context.Query("page_size"), 25)
		if pageSize > 100 {
			pageSize = 100
		}
		filters := []string{}
		args := []any{}
		addFilter := func(expression, value string) {
			if strings.TrimSpace(value) != "" {
				args = append(args, strings.TrimSpace(value))
				filters = append(filters, expression+" $"+strconv.Itoa(len(args)))
			}
		}
		addWildcardFilter := func(expression, value string) {
			if strings.TrimSpace(value) != "" {
				addFilter(expression, "%"+value+"%")
			}
		}
		addWildcardFilter("case_id ILIKE", context.Query("case_id"))
		addWildcardFilter("description ILIKE", context.Query("q"))
		addFilter("case_type =", strings.ToUpper(context.Query("case_type")))
		addFilter("modality =", strings.ToUpper(context.Query("modality")))
		if year := strings.TrimSpace(context.Query("year")); year != "" && caseYearPattern.MatchString(year) {
			addFilter("case_id ~", `(^|\.)`+year+`(\.|$)`)
		}
		if state := strings.ToUpper(strings.TrimSpace(context.Query("state"))); state != "" {
			args = append(args, state)
			filters = append(filters, "unit_id IN ("+unitStates+" SELECT id FROM unit_states WHERE uf = $"+strconv.Itoa(len(args))+")")
		}
		user := currentUser(context)
		every, err := distribution.SeesEveryCase(context.Request.Context(), db, user)
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not check case access"})
			return
		}
		if !every {
			args = append(args, user.ID)
			filters = append(filters, assignedToFilter("$"+strconv.Itoa(len(args))))
		}
		where := ""
		if len(filters) > 0 {
			where = " WHERE " + strings.Join(filters, " AND ")
		}
		var total int
		if err := db.QueryRowContext(context.Request.Context(), "SELECT COUNT(*) FROM biometric_cases"+where, args...).Scan(&total); err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not count biometric cases"})
			return
		}
		args = append(args, pageSize, (page-1)*pageSize)
		rows, err := db.QueryContext(context.Request.Context(), `SELECT case_id, case_type, modality, description FROM biometric_cases`+where+` ORDER BY case_id LIMIT $`+strconv.Itoa(len(args)-1)+` OFFSET $`+strconv.Itoa(len(args)), args...)
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not list biometric cases"})
			return
		}
		defer rows.Close()
		items := []cases.Case{}
		for rows.Next() {
			var item cases.Case
			if err := rows.Scan(&item.CaseID, &item.CaseType, &item.Modality, &item.Description); err != nil {
				context.JSON(http.StatusInternalServerError, gin.H{"error": "could not read biometric case"})
				return
			}
			items = append(items, item)
		}
		if err := rows.Err(); err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not read biometric cases"})
			return
		}
		totalPages := (total + pageSize - 1) / pageSize
		context.JSON(http.StatusOK, ListResponse{Items: items, Total: total, Page: page, PageSize: pageSize, TotalPages: totalPages})
	}
}

// ListCaseYearsHandler returns the distinct years found across all case_ids,
// newest first, for populating the "filter by year" dropdown. A year is any
// dot-separated case_id segment matching caseYearPattern, regardless of its
// position -- case_ids from different sources place the year differently
// (e.g. "01.2026.01.SRMG.00270" vs. "2026.0001").
func ListCaseYearsHandler(db *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		if db == nil {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
			return
		}
		where, args := "", []any{}
		user := currentUser(context)
		every, err := distribution.SeesEveryCase(context.Request.Context(), db, user)
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not check case access"})
			return
		}
		if !every {
			where, args = " WHERE "+assignedToFilter("$1"), []any{user.ID}
		}
		rows, err := db.QueryContext(context.Request.Context(), `
			SELECT DISTINCT seg
			FROM (SELECT unnest(string_to_array(case_id, '.')) AS seg FROM biometric_cases`+where+`) parts
			WHERE seg ~ '^(19|20)[0-9]{2}$'
			ORDER BY seg DESC`, args...)
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not list case years"})
			return
		}
		defer rows.Close()
		years := []string{}
		for rows.Next() {
			var year string
			if err := rows.Scan(&year); err != nil {
				context.JSON(http.StatusInternalServerError, gin.H{"error": "could not read case years"})
				return
			}
			years = append(years, year)
		}
		if err := rows.Err(); err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not read case years"})
			return
		}
		context.JSON(http.StatusOK, gin.H{"years": years})
	}
}

// ListCaseStatesHandler returns the states of the units responsible for the cases the user may list, for populating
// the "filter by state" dropdown.
func ListCaseStatesHandler(db *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		if db == nil {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
			return
		}
		where, args := "", []any{}
		user := currentUser(context)
		every, err := distribution.SeesEveryCase(context.Request.Context(), db, user)
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not check case access"})
			return
		}
		if !every {
			where, args = " WHERE "+assignedToFilter("$1"), []any{user.ID}
		}
		rows, err := db.QueryContext(context.Request.Context(), unitStates+`
			SELECT DISTINCT uf FROM unit_states
			WHERE uf IS NOT NULL AND id IN (SELECT unit_id FROM biometric_cases`+where+`)
			ORDER BY uf`, args...)
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not list case states"})
			return
		}
		defer rows.Close()
		states := []string{}
		for rows.Next() {
			var state string
			if err := rows.Scan(&state); err != nil {
				context.JSON(http.StatusInternalServerError, gin.H{"error": "could not read case states"})
				return
			}
			states = append(states, state)
		}
		if err := rows.Err(); err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not read case states"})
			return
		}
		context.JSON(http.StatusOK, gin.H{"states": states})
	}
}

func parsePositiveInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		return fallback
	}
	return parsed
}
