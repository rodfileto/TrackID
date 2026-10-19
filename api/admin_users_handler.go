package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/rodfileto/trackid/auth"
)

// ListUsersHandler lists every account, deactivated ones included.
func ListUsersHandler(repo *auth.Repository) gin.HandlerFunc {
	return func(context *gin.Context) {
		users, err := repo.ListUsers(context.Request.Context())
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not list users"})
			return
		}
		context.JSON(http.StatusOK, users)
	}
}

type createUserRequest struct {
	Nome                string           `json:"nome" binding:"required"`
	UltimoNome          string           `json:"ultimo_nome" binding:"required"`
	Matricula           string           `json:"matricula" binding:"required"`
	Cargo               string           `json:"cargo" binding:"required"`
	Username            string           `json:"username" binding:"required"`
	Password            string           `json:"password" binding:"required"`
	AccountRole         auth.AccountRole `json:"account_role"`
	ExaminesFingerprint bool             `json:"examines_fingerprint"`
	ExaminesFacial      bool             `json:"examines_facial"`
	IntelligenceAnalyst bool             `json:"intelligence_analyst"`
	Coordinator         bool             `json:"coordinator"`
}

// CreateUserHandler creates an account; account_role defaults to USER. Who may create which
// account is auth.CheckAccountChange.
func CreateUserHandler(repo *auth.Repository, emailDomain string) gin.HandlerFunc {
	return func(context *gin.Context) {
		var request createUserRequest
		if err := context.ShouldBindJSON(&request); err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid user data"})
			return
		}
		if request.AccountRole == "" {
			request.AccountRole = auth.AccountRoleUser
		}
		access := auth.Access{AccountRole: request.AccountRole, ExaminesFingerprint: request.ExaminesFingerprint, ExaminesFacial: request.ExaminesFacial,
			IntelligenceAnalyst: request.IntelligenceAnalyst, Coordinator: request.Coordinator, IsActive: true}
		if err := auth.CheckAccountChange(currentUser(context), nil, access); err != nil {
			accountChangeError(context, err)
			return
		}
		user, err := repo.Create(context.Request.Context(), auth.NewUser{
			Nome: request.Nome, UltimoNome: request.UltimoNome, Matricula: request.Matricula, Cargo: request.Cargo,
			Username: request.Username, Password: request.Password, Access: access,
		}, emailDomain)
		if err != nil {
			createUserError(context, err)
			return
		}
		context.JSON(http.StatusCreated, user)
	}
}

type updateUserAccessRequest struct {
	AccountRole         *auth.AccountRole `json:"account_role"`
	ExaminesFingerprint *bool             `json:"examines_fingerprint"`
	ExaminesFacial      *bool             `json:"examines_facial"`
	IntelligenceAnalyst *bool             `json:"intelligence_analyst"`
	Coordinator         *bool             `json:"coordinator"`
	IsActive            *bool             `json:"is_active"`
}

// UpdateUserAccessHandler changes an account's role, capabilities or active flag; fields left out
// keep their value.
func UpdateUserAccessHandler(repo *auth.Repository) gin.HandlerFunc {
	return func(context *gin.Context) {
		id, err := strconv.ParseInt(context.Param("userId"), 10, 64)
		if err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
			return
		}
		var request updateUserAccessRequest
		if err := context.ShouldBindJSON(&request); err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid access data"})
			return
		}
		target, err := repo.FindByID(context.Request.Context(), id)
		if errors.Is(err, sql.ErrNoRows) {
			context.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not load user"})
			return
		}
		access := auth.Access{AccountRole: target.AccountRole, ExaminesFingerprint: target.ExaminesFingerprint, ExaminesFacial: target.ExaminesFacial,
			IntelligenceAnalyst: target.IntelligenceAnalyst, Coordinator: target.Coordinator, IsActive: target.IsActive}
		if request.AccountRole != nil {
			access.AccountRole = *request.AccountRole
		}
		if request.ExaminesFingerprint != nil {
			access.ExaminesFingerprint = *request.ExaminesFingerprint
		}
		if request.ExaminesFacial != nil {
			access.ExaminesFacial = *request.ExaminesFacial
		}
		if request.IntelligenceAnalyst != nil {
			access.IntelligenceAnalyst = *request.IntelligenceAnalyst
		}
		if request.Coordinator != nil {
			access.Coordinator = *request.Coordinator
		}
		if request.IsActive != nil {
			access.IsActive = *request.IsActive
		}
		if err := auth.CheckAccountChange(currentUser(context), &target, access); err != nil {
			accountChangeError(context, err)
			return
		}
		user, err := repo.UpdateAccess(context.Request.Context(), id, access)
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not update user"})
			return
		}
		context.JSON(http.StatusOK, user)
	}
}

func accountChangeError(context *gin.Context, err error) {
	status := http.StatusForbidden
	if errors.Is(err, auth.ErrInvalidRole) {
		status = http.StatusBadRequest
	}
	context.JSON(status, gin.H{"error": err.Error()})
}

func createUserError(context *gin.Context, err error) {
	switch {
	case errors.Is(err, auth.ErrPasswordTooShort):
		context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case strings.Contains(err.Error(), "users_matricula_key"):
		context.JSON(http.StatusConflict, gin.H{"error": "matricula already exists"})
	case strings.Contains(err.Error(), "users_username_key") || strings.Contains(err.Error(), "users_email_key"):
		context.JSON(http.StatusConflict, gin.H{"error": "username already exists"})
	default:
		context.JSON(http.StatusInternalServerError, gin.H{"error": "could not create user"})
	}
}
