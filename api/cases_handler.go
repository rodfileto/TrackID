package api

import (
	"database/sql"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/rodfileto/trackid/cases"
)

// caseYearPattern matches a single case_id segment (case_ids are dot-separated,
// e.g. "01.2026.01.SRMG.00270" or "2026.0001") that looks like a year, whichever
// position it appears in.
var caseYearPattern = regexp.MustCompile(`^(19|20)\d{2}$`)

type ListResponse struct {
	Items      []cases.Case `json:"items"`
	Total      int          `json:"total"`
	Page       int          `json:"page"`
	PageSize   int          `json:"pageSize"`
	TotalPages int          `json:"totalPages"`
}

type createCaseRequest struct {
	CaseType    string `json:"caseType"`
	Modality    string `json:"modality"`
	Description string `json:"description"`
}

func CreateCaseHandler(db *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		if db == nil {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
			return
		}
		var request createCaseRequest
		if err := context.ShouldBindJSON(&request); err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}
		created, err := cases.Create(context.Request.Context(), db, cases.CreateInput{
			CaseType:    request.CaseType,
			Modality:    request.Modality,
			Description: request.Description,
		})
		if err != nil {
			var validationErr cases.ValidationError
			if errors.As(err, &validationErr) {
				context.JSON(http.StatusBadRequest, gin.H{"error": validationErr.Error()})
				return
			}
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not create case"})
			return
		}
		context.JSON(http.StatusCreated, created)
	}
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
		rows, err := db.QueryContext(context.Request.Context(), `
			SELECT DISTINCT seg
			FROM (SELECT unnest(string_to_array(case_id, '.')) AS seg FROM biometric_cases) parts
			WHERE seg ~ '^(19|20)[0-9]{2}$'
			ORDER BY seg DESC`)
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

func parsePositiveInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		return fallback
	}
	return parsed
}
