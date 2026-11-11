package reports

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/rodfileto/trackid/collections"
	"github.com/rodfileto/trackid/db"
)

// The scene a scene examination's laudo describes (docs/scene-examination-plan.md, decision 7): the
// occurrence's place and time, the case's collections with their team, and its vestiges with
// their seals. Core lays no scene laudo out; a deployment's layout (RegisterLayout) reads this.

// SceneInfo is a case's scene.
type SceneInfo struct {
	Occurrence  *OccurrenceInfo  `json:"occurrence,omitempty"`
	Collections []CollectionInfo `json:"collections"`
	Vestiges    []VestigeInfo    `json:"vestiges"`
}

// OccurrenceInfo is the occurrence a scene belongs to.
type OccurrenceInfo struct {
	Number        string     `json:"number,omitempty"` // TrackID number and/or police number
	InquiryNumber string     `json:"inquiryNumber,omitempty"`
	Types         string     `json:"types,omitempty"` // occurrence type codes, comma-separated
	Description   string     `json:"description,omitempty"`
	OccurredFrom  *time.Time `json:"occurredFrom,omitempty"`
	OccurredTo    *time.Time `json:"occurredTo,omitempty"`
	PlaceName     string     `json:"placeName,omitempty"`
	PlaceType     string     `json:"placeType,omitempty"`
	Address       string     `json:"address,omitempty"`
	Neighbourhood string     `json:"neighbourhood,omitempty"`
	Municipality  string     `json:"municipality,omitempty"`
	UF            string     `json:"uf,omitempty"`
	Lon           *float64   `json:"lon,omitempty"`
	Lat           *float64   `json:"lat,omitempty"`
	Precision     string     `json:"precision,omitempty"`
}

// CollectionInfo is one scene examined on call.
type CollectionInfo struct {
	Sequence         int16      `json:"sequence"`
	CalledAt         *time.Time `json:"calledAt,omitempty"`
	CollectedFrom    time.Time  `json:"collectedFrom"`
	CollectedTo      *time.Time `json:"collectedTo,omitempty"`
	Collector        Person     `json:"collector"`
	Team             []Person   `json:"team"`
	OtherOfficers    string     `json:"otherOfficers,omitempty"`
	NothingCollected bool       `json:"nothingCollected"`
	Notes            string     `json:"notes,omitempty"`
	Place            string     `json:"place,omitempty"`   // when the scene is not the occurrence's place
	Request          string     `json:"request,omitempty"` // the document of the request that formalized it
	AudioNotes       []FileInfo `json:"audioNotes"`
}

// VestigeInfo is one physical item of the case, its origin and its seals.
type VestigeInfo struct {
	Label       string     `json:"label"` // "V1"
	Description string     `json:"description"`
	Collection  int16      `json:"collection,omitempty"` // the collection's sequence, when collected
	Received    bool       `json:"received"`             // sent in by a request
	Seals       []SealInfo `json:"seals"`
}

// SealInfo is one seal a vestige was kept under.
type SealInfo struct {
	Number    string     `json:"number"`
	AppliedAt time.Time  `json:"appliedAt"`
	AppliedBy string     `json:"appliedBy,omitempty"`
	BrokenAt  *time.Time `json:"brokenAt,omitempty"`
	BrokenBy  string     `json:"brokenBy,omitempty"`
}

// buildScene reads a case's scene. It is nil for a case with no collection.
func buildScene(ctx context.Context, q *db.Queries, caseID string, occurrenceID sql.NullInt64, occurrenceNumber, occurrenceTypes string) (*SceneInfo, error) {
	origins, err := collections.ListWith(ctx, q, caseID)
	if err != nil {
		return nil, err
	}
	if len(origins.Collections) == 0 {
		return nil, nil
	}
	scene := &SceneInfo{Collections: []CollectionInfo{}, Vestiges: []VestigeInfo{}}
	if occurrenceID.Valid {
		o, err := q.GetOccurrenceForReport(ctx, occurrenceID.Int64)
		if err != nil {
			return nil, err
		}
		info := &OccurrenceInfo{Number: occurrenceNumber, InquiryNumber: o.InquiryNumber.String, Types: occurrenceTypes,
			Description: o.Description, PlaceName: o.PlaceName.String, PlaceType: o.PlaceType.String, Address: o.Address.String,
			Neighbourhood: o.Neighbourhood.String, Municipality: o.Municipality, UF: o.LocationUf.String, Precision: o.Precision.String}
		if o.OccurredFrom.Valid {
			info.OccurredFrom = &o.OccurredFrom.Time
		}
		if o.OccurredTo.Valid {
			info.OccurredTo = &o.OccurredTo.Time
		}
		if o.HasPlace {
			info.Lon, info.Lat = &o.Lon, &o.Lat
		}
		scene.Occurrence = info
	}

	// The people: collectors and team members, by id.
	ids := map[int64]bool{}
	for _, c := range origins.Collections {
		ids[c.CollectedByID] = true
		for _, m := range c.Team {
			ids[m.ID] = true
		}
	}
	list := make([]int64, 0, len(ids))
	for id := range ids {
		list = append(list, id)
	}
	people := map[int64]Person{}
	rows, err := q.GetUserNames(ctx, list)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		people[r.ID] = toPerson(r.Username, r.FirstName, r.LastName, r.StaffNumber, r.JobTitle)
	}
	sequenceOf := map[int64]int16{}
	for _, c := range origins.Collections {
		sequenceOf[c.ID] = c.Sequence
		info := CollectionInfo{Sequence: c.Sequence, CalledAt: c.CalledAt, CollectedFrom: c.CollectedFrom, CollectedTo: c.CollectedTo,
			Collector: people[c.CollectedByID], Team: []Person{}, OtherOfficers: c.OtherOfficers, NothingCollected: c.NothingCollected,
			Notes: c.Notes, Place: strings.TrimSpace(c.Place), AudioNotes: []FileInfo{}}
		if info.Collector.Username == "" {
			info.Collector = Person{Username: c.CollectedBy, Name: c.CollectedBy}
		}
		for _, m := range c.Team {
			if p, ok := people[m.ID]; ok {
				info.Team = append(info.Team, p)
			}
		}
		if c.Request != nil {
			info.Request = c.Request.DocumentReference
		}
		for _, a := range c.AudioNotes {
			info.AudioNotes = append(info.AudioNotes, FileInfo{Filename: a.Filename, SHA256: a.SHA256})
		}
		scene.Collections = append(scene.Collections, info)
	}
	for _, v := range origins.Vestiges {
		info := VestigeInfo{Label: v.Label, Description: v.Description, Received: v.RequestID != nil, Seals: []SealInfo{}}
		if v.CollectionID != nil {
			info.Collection = sequenceOf[*v.CollectionID]
		}
		for _, s := range v.Seals {
			info.Seals = append(info.Seals, SealInfo{Number: s.Number, AppliedAt: s.AppliedAt, AppliedBy: s.AppliedBy, BrokenAt: s.BrokenAt,
				BrokenBy: s.BrokenBy})
		}
		scene.Vestiges = append(scene.Vestiges, info)
	}
	return scene, nil
}
