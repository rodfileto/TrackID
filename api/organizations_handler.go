package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/rodfileto/trackid/auth"
	"github.com/rodfileto/trackid/organizations"
)

// Organizations, units and who works where (docs/organizations-plan.md, phase 1). Every admin reads them
// and manages units and assignments; only a SUPER_ADMIN creates, changes or deletes organizations.

// RequireSuperAdmin lets through only a SUPER_ADMIN. It runs after RequireAdmin.
func RequireSuperAdmin() gin.HandlerFunc {
	return func(context *gin.Context) {
		if currentUser(context).AccountRole != auth.AccountRoleSuperAdmin {
			context.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "super admin access required"})
			return
		}
		context.Next()
	}
}

func organizationsError(context *gin.Context, err error) {
	var invalid organizations.ValidationError
	switch {
	case errors.As(err, &invalid):
		context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, organizations.ErrNotFound):
		context.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, organizations.ErrInUse), errors.Is(err, organizations.ErrDuplicate), errors.Is(err, organizations.ErrOverlap):
		context.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	default:
		context.JSON(http.StatusInternalServerError, gin.H{"error": "could not save"})
	}
}

func idParam(context *gin.Context, name string) (int64, bool) {
	id, err := strconv.ParseInt(context.Param(name), 10, 64)
	if err != nil {
		context.JSON(http.StatusBadRequest, gin.H{"error": "invalid " + name})
		return 0, false
	}
	return id, true
}

// ListOrganizationsHandler lists every organization, inactive ones included.
func ListOrganizationsHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		list, err := organizations.Service{DB: sqlDB}.ListOrganizations(context.Request.Context())
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not list organizations"})
			return
		}
		context.JSON(http.StatusOK, list)
	}
}

// CreateOrganizationHandler creates an organization with its root unit.
func CreateOrganizationHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		var in organizations.OrganizationInput
		if err := context.ShouldBindJSON(&in); err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid organization"})
			return
		}
		id, err := organizations.Service{DB: sqlDB}.CreateOrganization(context.Request.Context(), in)
		if err != nil {
			organizationsError(context, err)
			return
		}
		context.JSON(http.StatusCreated, gin.H{"id": id})
	}
}

func UpdateOrganizationHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		id, ok := idParam(context, "organizationId")
		if !ok {
			return
		}
		var in organizations.OrganizationInput
		if err := context.ShouldBindJSON(&in); err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid organization"})
			return
		}
		if err := (organizations.Service{DB: sqlDB}).UpdateOrganization(context.Request.Context(), id, in); err != nil {
			organizationsError(context, err)
			return
		}
		context.Status(http.StatusNoContent)
	}
}

func DeleteOrganizationHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		id, ok := idParam(context, "organizationId")
		if !ok {
			return
		}
		if err := (organizations.Service{DB: sqlDB}).DeleteOrganization(context.Request.Context(), id); err != nil {
			organizationsError(context, err)
			return
		}
		context.Status(http.StatusNoContent)
	}
}

// ListUnitsHandler lists every unit of every organization; the page builds the trees.
func ListUnitsHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		list, err := organizations.Service{DB: sqlDB}.ListUnits(context.Request.Context())
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not list units"})
			return
		}
		context.JSON(http.StatusOK, list)
	}
}

func CreateUnitHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		var in organizations.UnitInput
		if err := context.ShouldBindJSON(&in); err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid unit"})
			return
		}
		id, err := organizations.Service{DB: sqlDB}.CreateUnit(context.Request.Context(), in)
		if err != nil {
			organizationsError(context, err)
			return
		}
		context.JSON(http.StatusCreated, gin.H{"id": id})
	}
}

func UpdateUnitHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		id, ok := idParam(context, "unitId")
		if !ok {
			return
		}
		var in organizations.UnitInput
		if err := context.ShouldBindJSON(&in); err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid unit"})
			return
		}
		if err := (organizations.Service{DB: sqlDB}).UpdateUnit(context.Request.Context(), id, in); err != nil {
			organizationsError(context, err)
			return
		}
		context.Status(http.StatusNoContent)
	}
}

func DeleteUnitHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		id, ok := idParam(context, "unitId")
		if !ok {
			return
		}
		if err := (organizations.Service{DB: sqlDB}).DeleteUnit(context.Request.Context(), id); err != nil {
			organizationsError(context, err)
			return
		}
		context.Status(http.StatusNoContent)
	}
}

// ListUnitAssignmentsHandler lists every user's postings and missions, past ones included.
func ListUnitAssignmentsHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		list, err := organizations.Service{DB: sqlDB}.ListAssignments(context.Request.Context())
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not list assignments"})
			return
		}
		context.JSON(http.StatusOK, list)
	}
}

func CreateUnitAssignmentHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		var in organizations.AssignmentInput
		if err := context.ShouldBindJSON(&in); err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid assignment"})
			return
		}
		id, err := organizations.Service{DB: sqlDB}.CreateAssignment(context.Request.Context(), in, currentUser(context).ID)
		if err != nil {
			organizationsError(context, err)
			return
		}
		context.JSON(http.StatusCreated, gin.H{"id": id})
	}
}

func UpdateUnitAssignmentHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		id, ok := idParam(context, "assignmentId")
		if !ok {
			return
		}
		var in organizations.AssignmentInput
		if err := context.ShouldBindJSON(&in); err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid assignment"})
			return
		}
		if err := (organizations.Service{DB: sqlDB}).UpdateAssignment(context.Request.Context(), id, in); err != nil {
			organizationsError(context, err)
			return
		}
		context.Status(http.StatusNoContent)
	}
}

func DeleteUnitAssignmentHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		id, ok := idParam(context, "assignmentId")
		if !ok {
			return
		}
		if err := (organizations.Service{DB: sqlDB}).DeleteAssignment(context.Request.Context(), id); err != nil {
			organizationsError(context, err)
			return
		}
		context.Status(http.StatusNoContent)
	}
}
