package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/rodfileto/trackid/analyses"
	"github.com/rodfileto/trackid/auth"
	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/distribution"
)

// Middleware authenticates the request's Bearer token and, on success, sets the
// "userID" and "username" values on the context.
func Middleware(jwtSecret string) gin.HandlerFunc {
	return func(context *gin.Context) {
		header := context.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") || jwtSecret == "" {
			context.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing or invalid token"})
			return
		}
		token, err := jwt.Parse(strings.TrimPrefix(header, "Bearer "), func(token *jwt.Token) (interface{}, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, errors.New("unexpected signing method")
			}
			return []byte(jwtSecret), nil
		})
		if err != nil || !token.Valid {
			context.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			return
		}
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			context.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token claims"})
			return
		}
		context.Set("userID", claims["sub"])
		if username, ok := claims["username"].(string); ok {
			context.Set("username", username)
		}
		context.Next()
	}
}

// currentUserKey is the context key ActiveUser stores the signed-in auth.UserProfile under.
const currentUserKey = "currentUser"

// ActiveUser runs after Middleware: it loads the token's user and refuses one that no longer
// exists or was deactivated, so deactivating an account takes effect on its next request
// rather than when its token expires. Handlers read the profile with currentUser.
func ActiveUser(repo *auth.Repository) gin.HandlerFunc {
	return func(context *gin.Context) {
		if repo == nil {
			context.AbortWithStatus(http.StatusServiceUnavailable)
			return
		}
		userID, _ := context.Get("userID")
		subject, _ := userID.(string)
		id, err := strconv.ParseInt(subject, 10, 64)
		if err != nil {
			context.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token claims"})
			return
		}
		user, err := repo.FindByID(context.Request.Context(), id)
		if errors.Is(err, sql.ErrNoRows) {
			context.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
			return
		}
		if err != nil {
			context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "could not load user"})
			return
		}
		if !user.IsActive {
			context.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "account is deactivated"})
			return
		}
		context.Set(currentUserKey, user)
		context.Next()
	}
}

// RequireAdmin lets through only an ADMIN or SUPER_ADMIN: account management, nothing else. It
// runs after ActiveUser.
func RequireAdmin() gin.HandlerFunc {
	return func(context *gin.Context) {
		if !currentUser(context).AccountRole.IsAdmin() {
			context.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "admin access required"})
			return
		}
		context.Next()
	}
}

func currentUser(context *gin.Context) auth.UserProfile {
	user, _ := context.Get(currentUserKey)
	profile, _ := user.(auth.UserProfile)
	return profile
}

// blindRoutes are the cases/:caseId routes left to a user who works one of the case's analyses
// blind (distribution.BlindIn): the workspace and its pairs' images.
var blindRoutes = map[string]bool{
	"/api/cases/:caseId/analyses/:analysisId/pairs":       true,
	"/api/cases/:caseId/analyses/:analysisId/decisions":   true,
	"/api/cases/:caseId/analyses/:analysisId/submit":      true,
	"/api/cases/:caseId/analyses/:analysisId/pairs/image": true,
}

// intelligenceCaseRoutes are the cases/:caseId routes that change the case but are intelligence
// work, not examination: RequireIntelligence guards them, so CaseAccess does not ask the user to
// hold an analysis there.
var intelligenceCaseRoutes = map[string]bool{
	"/api/cases/:caseId/occurrence": true,
}

// blindMessage is the refusal while a user works an analysis blind.
const blindMessage = "blind work: only the analysis is open to you until you submit it"

// CaseAccess guards every route under cases/:caseId (distribution.Rights): it runs after
// ActiveUser and lets a user read a case where they hold an analysis, or any case as an analyst or
// coordinator; change it only where they hold an analysis; and, while they work one of its
// analyses blind, reach blindRoutes only -- whatever their account role or capabilities.
func CaseAccess(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		if sqlDB == nil {
			context.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
			return
		}
		rights, err := distribution.Rights(context.Request.Context(), sqlDB, currentUser(context), context.Param("caseId"))
		path := context.FullPath()
		reads := context.Request.Method == http.MethodGet || context.Request.Method == http.MethodHead
		switch {
		case errors.Is(err, analyses.ErrCaseNotFound):
			context.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "case not found"})
		case err != nil:
			context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "could not check case access"})
		case !rights.Read:
			context.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": distribution.ErrNotAssigned.Error()})
		case rights.Blind && !blindRoutes[path]:
			context.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": blindMessage})
		case !reads && !rights.Write && !intelligenceCaseRoutes[path]:
			context.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "only an examiner holding one of its analyses can change this case"})
		default:
			// Reading a case one works on is the work itself; anyone else's read is logged.
			if !rights.Holds && !recordAccess(context, sqlDB, accessCase, context.Param("caseId")) {
				return
			}
			context.Next()
		}
	}
}

// Kinds of access_log rows.
const (
	accessPerson     = "PERSON"
	accessFaceSearch = "FACE_SEARCH"
	accessCase       = "CASE"
)

// recordAccess writes one access_log row for the signed-in user's request (subject "" for none).
// A read that cannot be recorded is not served: it aborts with 500 and returns false.
func recordAccess(context *gin.Context, sqlDB *sql.DB, kind, subject string) bool {
	user := currentUser(context)
	err := db.New(sqlDB).InsertAccessLog(context.Request.Context(), db.InsertAccessLogParams{
		UserID: user.ID, Username: user.Username, Kind: kind,
		Subject: sql.NullString{String: subject, Valid: subject != ""}, Route: context.FullPath(),
	})
	if err != nil {
		context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "could not record the access"})
		return false
	}
	return true
}

// LogAccess records each request of a route in access_log (recordAccess), its subject the route
// parameter param ("" for none): the intelligence routes on a person's history and face search.
// It runs after the route's guard.
func LogAccess(sqlDB *sql.DB, kind, param string) gin.HandlerFunc {
	return func(context *gin.Context) {
		subject := ""
		if param != "" {
			subject = context.Param(param)
		}
		if recordAccess(context, sqlDB, kind, subject) {
			context.Next()
		}
	}
}

// requireCapability lets through a user who has the capability (has) and works nothing blind
// (distribution.BlindAnywhere). It runs after ActiveUser.
func requireCapability(sqlDB *sql.DB, has func(auth.UserProfile) bool, refusal string) gin.HandlerFunc {
	return func(context *gin.Context) {
		user := currentUser(context)
		if !has(user) {
			context.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": refusal})
			return
		}
		if sqlDB == nil {
			context.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
			return
		}
		blind, err := distribution.BlindAnywhere(context.Request.Context(), sqlDB, user.ID)
		if err != nil {
			context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "could not check access"})
			return
		}
		if blind {
			context.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": blindMessage})
			return
		}
		context.Next()
	}
}

// RequireIntelligence guards the intelligence routes: the full person page, face search,
// clusters, occurrences.
func RequireIntelligence(sqlDB *sql.DB) gin.HandlerFunc {
	return requireCapability(sqlDB, auth.UserProfile.IntelligenceAccess, "intelligence access required")
}

// RequireCoordinator guards distribution, reassignment and opening comparisons.
func RequireCoordinator(sqlDB *sql.DB) gin.HandlerFunc {
	return requireCapability(sqlDB, auth.UserProfile.Coordinates, "coordinator access required")
}

// RequirePersonIdentity guards person search by name and a person's identity (documents,
// registers, enrollment photos): examiners and analysts. Only analysts get the rest of the person
// page (RequireIntelligence).
func RequirePersonIdentity(sqlDB *sql.DB) gin.HandlerFunc {
	return requireCapability(sqlDB, func(u auth.UserProfile) bool {
		return u.IntelligenceAccess() || u.Examines(auth.ModalityFacial) || u.Examines(auth.ModalityFingerprint)
	}, "person search is for examiners and analysts")
}
