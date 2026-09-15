package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/rodfileto/trackid/cases"
)

type ListResponse struct {
	Items      []cases.Case `json:"items"`
	Total      int          `json:"total"`
	Page       int          `json:"page"`
	PageSize   int          `json:"pageSize"`
	TotalPages int          `json:"totalPages"`
}

type createCaseRequest struct {
	CaseType    string `json:"caseType"`
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
		where := ""
		if len(filters) > 0 {
			where = " WHERE " + strings.Join(filters, " AND ")
		}
		var total int
		if err := db.QueryRowContext(context.Request.Context(), "SELECT COUNT(*) FROM criminal_cases"+where, args...).Scan(&total); err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not count criminal cases"})
			return
		}
		args = append(args, pageSize, (page-1)*pageSize)
		rows, err := db.QueryContext(context.Request.Context(), `SELECT case_id, case_type, description FROM criminal_cases`+where+` ORDER BY case_id LIMIT $`+strconv.Itoa(len(args)-1)+` OFFSET $`+strconv.Itoa(len(args)), args...)
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not list criminal cases"})
			return
		}
		defer rows.Close()
		items := []cases.Case{}
		for rows.Next() {
			var item cases.Case
			if err := rows.Scan(&item.CaseID, &item.CaseType, &item.Description); err != nil {
				context.JSON(http.StatusInternalServerError, gin.H{"error": "could not read criminal case"})
				return
			}
			items = append(items, item)
		}
		if err := rows.Err(); err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not read criminal cases"})
			return
		}
		totalPages := (total + pageSize - 1) / pageSize
		context.JSON(http.StatusOK, ListResponse{Items: items, Total: total, Page: page, PageSize: pageSize, TotalPages: totalPages})
	}
}

func parsePositiveInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		return fallback
	}
	return parsed
}
