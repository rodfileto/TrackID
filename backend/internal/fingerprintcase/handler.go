package fingerprintcase

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

type ListResponse struct {
	Items      []Record `json:"items"`
	Total      int      `json:"total"`
	Page       int      `json:"page"`
	PageSize   int      `json:"pageSize"`
	TotalPages int      `json:"totalPages"`
}

func ListHandler(db *sql.DB) gin.HandlerFunc {
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
		addWildcardFilter("related_reference ILIKE", context.Query("reference"))
		addFilter("comparison_type =", strings.ToUpper(context.Query("comparison_type")))
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
		rows, err := db.QueryContext(context.Request.Context(), `SELECT case_id, case_type, description, responsible_user, comparison_type, related_reference, related_reference_kind FROM criminal_cases`+where+` ORDER BY case_id LIMIT $`+strconv.Itoa(len(args)-1)+` OFFSET $`+strconv.Itoa(len(args)), args...)
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not list criminal cases"})
			return
		}
		defer rows.Close()
		items := []Record{}
		for rows.Next() {
			var item Record
			var responsibleUser, comparisonType, relatedReference, relatedKind sql.NullString
			if err := rows.Scan(&item.CaseID, &item.CaseType, &item.Description, &responsibleUser, &comparisonType, &relatedReference, &relatedKind); err != nil {
				context.JSON(http.StatusInternalServerError, gin.H{"error": "could not read criminal case"})
				return
			}
			item.ResponsibleUser = nullStringPointer(responsibleUser)
			item.ComparisonType = nullStringPointer(comparisonType)
			item.RelatedReference = nullStringPointer(relatedReference)
			item.RelatedReferenceKind = nullStringPointer(relatedKind)
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

func nullStringPointer(value sql.NullString) *string {
	if !value.Valid || strings.TrimSpace(value.String) == "" {
		return nil
	}
	return &value.String
}
