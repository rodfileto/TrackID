package person

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"image"
	"strings"

	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/embedding"
)

// searchResultLimit caps how many identity_register matches SearchByName
// returns -- a name search is a disambiguation aid for an analyst who
// already has a name in mind, not a bulk export.
const searchResultLimit = 25

// faceSearchResultLimit caps how many identity_register matches
// SearchByFace returns, same rationale as searchResultLimit.
const faceSearchResultLimit = 25

// caseFaceSearchResultLimit caps how many distinct criminal cases
// SearchByFace returns.
const caseFaceSearchResultLimit = 25

// caseFaceSearchFetchLimit is how many raw (pre-dedup) QUESTIONED matches
// SearchByFace fetches before collapsing them down to
// caseFaceSearchResultLimit distinct cases. Needs headroom over that limit
// because the same case can supply several of the nearest rows -- once per
// matching trace.
const caseFaceSearchFetchLimit = 200

// ErrNoFaceDetected is returned by SearchByFace when no face could be found
// in the uploaded image to embed and search with.
var ErrNoFaceDetected = errors.New("person: no face detected in image")

// ErrUnsupportedImage is returned by SearchByFace when the uploaded image is
// in a format trackid-vision can't decode.
var ErrUnsupportedImage = errors.New("unsupported image format for face search (JPEG, PNG, GIF, BMP, TIFF, and WebP only)")

// supportedImageFormats mirrors cases.supportedImageFormats -- the decoders
// trackid-vision's vision package registers (see its image.go).
var supportedImageFormats = map[string]bool{
	"jpeg": true,
	"png":  true,
	"gif":  true,
	"bmp":  true,
	"tiff": true,
	"webp": true,
}

// SearchResult is one identity_register whose name matched a SearchByName
// query -- a candidate for a caller to disambiguate before opening that
// person's full Profile. The same person_id can appear in more than one
// SearchResult when the search term matches several of their registers
// (e.g. an alias, or the same name spelled differently across enrollments).
//
// CaseCounts is the "N facial / N fingerprint" badge callers show alongside
// each candidate -- how many distinct criminal cases this person is linked
// to (see RelatedCase), broken down by case_type. Empty when the person has
// no linked cases.
type SearchResult struct {
	PersonID       string          `json:"personId"`
	Name           string          `json:"name"`
	RegisterNumber string          `json:"registerNumber"`
	DocumentType   string          `json:"documentType"`
	DocumentNumber string          `json:"documentNumber"`
	CaseCounts     []CaseTypeCount `json:"caseCounts"`
}

// CaseTypeCount is how many distinct criminal cases a search result is
// linked to for one case_type ("FACIAL" or "FINGERPRINT").
type CaseTypeCount struct {
	CaseType string `json:"caseType"`
	Count    int64  `json:"count"`
}

// caseCountsByPerson batches SearchResult.CaseCounts for every id in
// personIDs (duplicates allowed) into one CountPersonCasesByType query,
// shared by SearchByName and SearchByFace instead of one query per row.
func caseCountsByPerson(ctx context.Context, q *db.Queries, personIDs []string) (map[string][]CaseTypeCount, error) {
	if len(personIDs) == 0 {
		return nil, nil
	}
	rows, err := q.CountPersonCasesByType(ctx, personIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]CaseTypeCount, len(rows))
	for _, r := range rows {
		out[r.PersonID] = append(out[r.PersonID], CaseTypeCount{CaseType: r.CaseType, Count: r.CaseCount})
	}
	return out, nil
}

// SearchByName finds every enrolled person with at least one identity
// register whose name contains query (case-insensitive substring match,
// backed by the trigram index in
// db/migrations/021_add_person_name_search.sql). Results are capped at
// searchResultLimit; an empty (after trimming) query returns no results
// rather than the whole table.
func SearchByName(ctx context.Context, sqlDB *sql.DB, query string) ([]SearchResult, error) {
	if sqlDB == nil {
		return nil, fmt.Errorf("person: nil db")
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}

	q := db.New(sqlDB)
	rows, err := q.SearchPersonsByName(ctx, db.SearchPersonsByNameParams{
		Name:  "%" + query + "%",
		Limit: searchResultLimit,
	})
	if err != nil {
		return nil, err
	}

	out := make([]SearchResult, 0, len(rows))
	personIDs := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, SearchResult{
			PersonID:       r.PersonID,
			Name:           r.Name,
			RegisterNumber: r.RegisterNumber,
			DocumentType:   r.DocumentType,
			DocumentNumber: r.DocumentNumber,
		})
		personIDs = append(personIDs, r.PersonID)
	}

	counts, err := caseCountsByPerson(ctx, q, personIDs)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].CaseCounts = counts[out[i].PersonID]
	}
	return out, nil
}

// FaceSearchResult is one enrolled identity_register whose KNOWN face
// embedding matched a SearchByFace query, ranked by Similarity (cosine
// similarity, 1 = identical direction; see FindNearestPersonsByFaceEmbedding
// in db/queries.sql). IdentityFileID is the enrollment photo the embedding
// was computed from, for a caller to render as a thumbnail (see
// DownloadIdentityFileHandler) -- the whole file is already just the face,
// no crop needed (same assumption embedding.ComputeForIdentityFeature makes).
type FaceSearchResult struct {
	SearchResult
	Similarity     float64 `json:"similarity"`
	IdentityFileID int64   `json:"identityFileId"`
	ContentType    string  `json:"contentType,omitempty"`
}

// ThumbnailBox is a face's bounding box within ThumbnailFileID's image, in
// that image's own pixel coordinates -- present only when the thumbnail file
// isn't already a tight crop of just the face (see CaseFaceSearchResult).
type ThumbnailBox struct {
	X1 float64 `json:"x1"`
	Y1 float64 `json:"y1"`
	X2 float64 `json:"x2"`
	Y2 float64 `json:"y2"`
}

// CaseFaceSearchResult is one criminal case whose evidence holds a QUESTIONED
// face trace matching a SearchByFace query, ranked by Similarity. TraceID
// names the best-scoring trace that put this case in the results -- a case
// can hold several matching traces, only the closest one is kept (see
// SearchByFace).
//
// ThumbnailFileID/ThumbnailBox tell a caller how to render that trace as a
// thumbnail, same source priority GetCodificationSource uses: when the
// trace already has its own dedicated face_crop file, ThumbnailFileID names
// it and ThumbnailBox is nil (nothing left to crop); otherwise
// ThumbnailFileID names the evidence file and ThumbnailBox is the trace's
// box within it. ThumbnailFileID is 0 when neither is available (e.g. the
// evidence file was later excluded) -- a caller shows a placeholder.
type CaseFaceSearchResult struct {
	CaseID          string        `json:"caseId"`
	CaseType        string        `json:"caseType"`
	Description     string        `json:"description"`
	TraceID         int64         `json:"traceId"`
	Similarity      float64       `json:"similarity"`
	ThumbnailFileID int64         `json:"thumbnailFileId,omitempty"`
	ThumbnailBox    *ThumbnailBox `json:"thumbnailBox,omitempty"`
}

// FaceSearchResults is SearchByFace's result: enrolled persons whose KNOWN
// face matched, and criminal cases whose evidence holds a QUESTIONED face
// trace that matched. The two are kept apart rather than merged into one
// ranked list -- a KNOWN and a QUESTIONED embedding aren't guaranteed
// comparable enough for their similarity scores to rank meaningfully against
// each other, only within their own kind.
type FaceSearchResults struct {
	Persons []FaceSearchResult     `json:"persons"`
	Cases   []CaseFaceSearchResult `json:"cases"`
}

// SearchByFace finds enrolled persons whose enrolled (KNOWN) face, and
// criminal cases whose evidence holds a QUESTIONED face trace, most resemble
// the face in imageData, using the same embedding model and
// feature_embeddings table cases.CompareFaces/biometricmatch already use.
// imageData is expected to be a photo where the face is the main subject
// (see embedding.EmbedFaceInBox); the highest-scoring face detected is used.
// Returns ErrUnsupportedImage if imageData isn't a decodable image, and
// ErrNoFaceDetected if no face could be found in it to search with.
func SearchByFace(ctx context.Context, sqlDB *sql.DB, vis embedding.FaceEmbedder, imageData []byte) (FaceSearchResults, error) {
	if sqlDB == nil {
		return FaceSearchResults{}, fmt.Errorf("person: nil db")
	}
	if vis == nil {
		return FaceSearchResults{}, fmt.Errorf("person: nil face vision")
	}

	img, format, err := image.Decode(bytes.NewReader(imageData))
	if err != nil || !supportedImageFormats[format] {
		return FaceSearchResults{}, ErrUnsupportedImage
	}

	vector, err := embedding.EmbedFaceInBox(vis, img, img.Bounds())
	if err != nil {
		return FaceSearchResults{}, fmt.Errorf("person: embed search face: %w", err)
	}
	if vector == nil {
		return FaceSearchResults{}, ErrNoFaceDetected
	}
	vectorText := embedding.VectorText(vector)
	q := db.New(sqlDB)

	personRows, err := q.FindNearestPersonsByFaceEmbedding(ctx, db.FindNearestPersonsByFaceEmbeddingParams{
		Embedding:     vectorText,
		EmbeddingType: embedding.EmbeddingType,
		ResultLimit:   faceSearchResultLimit,
	})
	if err != nil {
		return FaceSearchResults{}, err
	}

	persons := make([]FaceSearchResult, 0, len(personRows))
	for _, r := range personRows {
		distance, ok := r.Distance.(float64)
		if !ok {
			continue
		}
		persons = append(persons, FaceSearchResult{
			SearchResult: SearchResult{
				PersonID:       r.PersonID,
				Name:           r.Name,
				RegisterNumber: r.RegisterNumber,
				DocumentType:   r.DocumentType,
				DocumentNumber: r.DocumentNumber,
			},
			Similarity:     1 - distance,
			IdentityFileID: r.IdentityFileID,
			ContentType:    r.ContentType.String,
		})
	}

	personIDs := make([]string, len(persons))
	for i, p := range persons {
		personIDs[i] = p.PersonID
	}
	counts, err := caseCountsByPerson(ctx, q, personIDs)
	if err != nil {
		return FaceSearchResults{}, err
	}
	for i := range persons {
		persons[i].CaseCounts = counts[persons[i].PersonID]
	}

	caseRows, err := q.FindNearestCasesByFaceEmbedding(ctx, db.FindNearestCasesByFaceEmbeddingParams{
		Embedding:     vectorText,
		EmbeddingType: embedding.EmbeddingType,
		ResultLimit:   caseFaceSearchFetchLimit,
	})
	if err != nil {
		return FaceSearchResults{}, err
	}

	cases := make([]CaseFaceSearchResult, 0, caseFaceSearchResultLimit)
	seenCases := make(map[string]bool, caseFaceSearchResultLimit)
	for _, r := range caseRows {
		if len(cases) >= caseFaceSearchResultLimit {
			break
		}
		if seenCases[r.CaseID] {
			// caseRows is ordered nearest-first, so the first row seen for a
			// case is already its best-scoring trace.
			continue
		}
		distance, ok := r.Distance.(float64)
		if !ok {
			continue
		}
		seenCases[r.CaseID] = true
		result := CaseFaceSearchResult{
			CaseID:      r.CaseID,
			CaseType:    r.CaseType,
			Description: r.Description,
			TraceID:     r.CaseTraceID,
			Similarity:  1 - distance,
		}
		switch {
		case r.TraceCropFileID.Valid:
			result.ThumbnailFileID = r.TraceCropFileID.Int64
		case r.EvidenceFileID.Valid && r.BoxX1.Valid:
			result.ThumbnailFileID = r.EvidenceFileID.Int64
			result.ThumbnailBox = &ThumbnailBox{
				X1: r.BoxX1.Float64,
				Y1: r.BoxY1.Float64,
				X2: r.BoxX2.Float64,
				Y2: r.BoxY2.Float64,
			}
		}
		cases = append(cases, result)
	}

	return FaceSearchResults{Persons: persons, Cases: cases}, nil
}
