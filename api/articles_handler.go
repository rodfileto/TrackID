package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/rodfileto/trackid/articles"
	"github.com/rodfileto/trackid/db"
)

// Person articles (docs/person-article-plan.md): what intelligence analysts write about a person.
// Every route here is behind RequireIntelligence; the reads of a person's article, its history
// and its backlinks go to access_log as reads of the person.

// GetPersonArticleHandler returns a person's article: its current revision (null while nobody
// has written one) and its mentions as they read now (articles.Get).
func GetPersonArticleHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		article, err := articles.Get(c.Request.Context(), sqlDB, articles.PersonSubject(c.Param("personId")))
		if err != nil {
			articleError(c, err, "could not load the article")
			return
		}
		c.JSON(http.StatusOK, article)
	}
}

type saveArticleRequest struct {
	// BaseRevisionID is the revision the edit started from, null or 0 for a first article.
	BaseRevisionID *int64          `json:"baseRevisionId"`
	Body           json.RawMessage `json:"body"`
	EditSummary    string          `json:"editSummary"`
	// TemplateID is the generated draft the edit started from (GeneratePersonArticleTemplate).
	TemplateID int64 `json:"templateId"`
}

// SavePersonArticleHandler saves an edit of a person's article as its new revision
// (articles.Save) and returns the article as it now reads, with created false when the edit
// changed nothing.
//
//   - 409 when another analyst saved first: the response carries the article as it now is, for
//     the editor to merge;
//   - 422 for a body the schema refuses, or mentions of targets that do not exist ("missing").
func SavePersonArticleHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, articles.MaxBodyBytes+64<<10)
		var r saveArticleRequest
		if err := c.ShouldBindJSON(&r); err != nil || len(r.Body) == 0 || string(r.Body) == "null" {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "the article is too large"})
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}
		subject := articles.PersonSubject(c.Param("personId"))
		in := articles.SaveInput{Subject: subject, Body: r.Body, EditSummary: r.EditSummary, AuthorID: currentUser(c).ID, TemplateID: r.TemplateID}
		if r.BaseRevisionID != nil {
			in.BaseRevisionID = *r.BaseRevisionID
		}
		ctx := c.Request.Context()
		_, created, err := articles.Save(ctx, sqlDB, in)
		if errors.Is(err, articles.ErrConflict) {
			current, loadErr := articles.Get(ctx, sqlDB, subject)
			if loadErr != nil {
				articleError(c, loadErr, "could not load the article")
				return
			}
			c.JSON(http.StatusConflict, gin.H{"error": "the article changed since this edit began", "article": current})
			return
		}
		if err != nil {
			articleError(c, err, "could not save the article")
			return
		}
		article, err := articles.Get(ctx, sqlDB, subject)
		if err != nil {
			articleError(c, err, "could not load the article")
			return
		}
		c.JSON(http.StatusOK, gin.H{"article": article, "created": created})
	}
}

// GeneratePersonArticleTemplateHandler writes a first draft of a person's article from the
// records (articles.GenerateTemplate), logs it, and returns it for the editor; nothing is saved
// as a revision. The JSON body's optional "language" ("pt-BR" or "en") picks the wording.
func GeneratePersonArticleTemplateHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var r struct {
			Language string `json:"language"`
		}
		if c.Request.ContentLength > 0 {
			if err := c.ShouldBindJSON(&r); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
				return
			}
		}
		template, err := articles.GenerateTemplate(c.Request.Context(), sqlDB,
			articles.PersonSubject(c.Param("personId")), r.Language, currentUser(c).ID)
		if err != nil {
			articleError(c, err, "could not generate the draft")
			return
		}
		c.JSON(http.StatusCreated, template)
	}
}

// GetPersonLinkCountsHandler counts what the records link to a person by certainty
// (articles.PersonLinkCounts): cases confirmed, not reviewed, uncertain and excluded; clusters
// per layer; other identities and associates, confirmed or not. The article's infobox shows them
// beside the reviewed profile.
func GetPersonLinkCountsHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		counts, err := articles.PersonLinkCounts(c.Request.Context(), sqlDB, c.Param("personId"))
		if err != nil {
			articleError(c, err, "could not count the links")
			return
		}
		c.JSON(http.StatusOK, counts)
	}
}

// GetPersonArticleHistoryHandler lists a person's article revisions, newest first, without text.
func GetPersonArticleHistoryHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		history, err := articles.History(c.Request.Context(), sqlDB, articles.PersonSubject(c.Param("personId")))
		if err != nil {
			articleError(c, err, "could not load the article history")
			return
		}
		c.JSON(http.StatusOK, gin.H{"revisions": history})
	}
}

// GetPersonArticleRevisionHandler returns one revision of a person's article, text included.
func GetPersonArticleRevisionHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		number, err := strconv.Atoi(c.Param("number"))
		if err != nil || number <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid revision number"})
			return
		}
		rev, err := articles.GetRevision(c.Request.Context(), sqlDB, articles.PersonSubject(c.Param("personId")), number)
		if err != nil {
			articleError(c, err, "could not load the revision")
			return
		}
		c.JSON(http.StatusOK, rev)
	}
}

// GetPersonBacklinksHandler lists the other persons' articles that mention this person ("what
// links here").
func GetPersonBacklinksHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		subject := articles.PersonSubject(c.Param("personId"))
		links, err := articles.Backlinks(c.Request.Context(), sqlDB,
			[]articles.MentionKey{{Kind: articles.KindPerson, Ref: subject.Ref}}, &subject)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load backlinks"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"backlinks": links})
	}
}

// GetCaseBacklinksHandler lists the articles that mention a case, its occurrence or inquiry,
// its requests or its issued laudos (articles.CaseTargets).
func GetCaseBacklinksHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		targets, err := articles.CaseTargets(ctx, sqlDB, c.Param("caseId"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load backlinks"})
			return
		}
		if len(targets) == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "case not found"})
			return
		}
		links, err := articles.Backlinks(ctx, sqlDB, targets, nil)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load backlinks"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"backlinks": links})
	}
}

// SearchMentionsHandler is the @ menu's search (articles.Search): q is the text typed after the
// @ (and after a kind prefix, which the editor reads), kinds an optional comma-separated list
// of mention kinds, limit how many of each (default 5, at most 20).
func SearchMentionsHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var kinds []articles.Kind
		if raw := strings.TrimSpace(c.Query("kinds")); raw != "" {
			for _, part := range strings.Split(raw, ",") {
				kind, err := articles.ParseKind(strings.ToUpper(strings.TrimSpace(part)))
				if err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
				kinds = append(kinds, kind)
			}
		}
		limit := 0
		if raw := c.Query("limit"); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n <= 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit"})
				return
			}
			limit = n
		}
		targets, err := articles.Search(c.Request.Context(), db.New(sqlDB), c.Query("q"), kinds, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not search"})
			return
		}
		if targets == nil {
			targets = []articles.Target{}
		}
		c.JSON(http.StatusOK, gin.H{"results": targets})
	}
}

// articleError answers an articles error: 404 for an unknown person or revision, 422 for a body
// or mentions the article refuses, 500 (with fallback) otherwise.
func articleError(c *gin.Context, err error, fallback string) {
	var bodyErr *articles.BodyError
	var mentionErr *articles.MentionError
	switch {
	case errors.Is(err, articles.ErrSubjectNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "person not found"})
	case errors.Is(err, articles.ErrTemplateNotFound):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "the generated draft does not exist for this person"})
	case errors.Is(err, articles.ErrRevisionNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "revision not found"})
	case errors.As(err, &bodyErr):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": bodyErr.Reason})
	case errors.As(err, &mentionErr):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "some mentioned items do not exist", "missing": mentionErr.Missing})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": fallback})
	}
}
