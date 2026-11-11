// Package scenenotes keeps the notes an examiner writes at a scene (docs/scene-examination-plan.md).
// A note is written on a phone that may have no signal, kept on the device until it syncs, and kept
// here as a draft still: its author's, not a case. It becomes a scene collection through
// collections.Register, which takes its audio notes with it.
//
// A note and each of its audios have an id made on the device, so syncing twice changes nothing.
package scenenotes

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/rodfileto/trackid/auth"
	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/geocode"
)

// ValidationError is an input the caller can fix.
type ValidationError string

func (e ValidationError) Error() string { return string(e) }

var (
	ErrNotFound  = errors.New("scenenotes: note not found")
	ErrForbidden = errors.New("scenenotes: the note is someone else's")
	// ErrConverted is returned for a change to a note that is already a collection.
	ErrConverted = errors.New("scenenotes: the note is already a collection")
)

// MaxAudioBytes is the largest audio note taken: about an hour of speech as a phone records it.
const MaxAudioBytes = 64 << 20

// placeTypes are locations.place_type's values.
var placeTypes = []string{"PUBLIC_ROAD", "RESIDENCE", "COMMERCE", "BANK", "GOVERNMENT_OFFICE", "TRANSPORT", "RURAL", "OTHER"}

// Store keeps files (storage.Client).
type Store interface {
	Upload(ctx context.Context, objectKey string, content []byte, contentType string) (string, error)
	Download(ctx context.Context, storageRef string) ([]byte, error)
}

// Service is the notes' dependencies. Geocoder may be nil: a note's point then gets no address.
type Service struct {
	DB       *sql.DB
	Store    Store
	Geocoder geocode.Geocoder
}

// Input is a note as the device sends it.
type Input struct {
	ClientID      string     `json:"clientId"`
	PlaceType     string     `json:"placeType"`
	PlaceName     string     `json:"placeName"`
	Lon           *float64   `json:"lon"`
	Lat           *float64   `json:"lat"`
	Address       string     `json:"address"`
	Neighbourhood string     `json:"neighbourhood"`
	CalledAt      *time.Time `json:"calledAt"`
	ExaminedAt    *time.Time `json:"examinedAt"`
	OccurredAt    *time.Time `json:"occurredAt"`
	Team          []int64    `json:"team"`
	OtherOfficers string     `json:"otherOfficers"`
	Notes         string     `json:"notes"`
	WrittenAt     time.Time  `json:"writtenAt"`
}

// Member is a user of a note's team.
type Member struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
}

// Audio is an audio note.
type Audio struct {
	ID          int64      `json:"id"`
	ClientID    string     `json:"clientId"`
	Title       string     `json:"title,omitempty"`
	ContentType string     `json:"contentType"`
	SizeBytes   int64      `json:"sizeBytes"`
	SHA256      string     `json:"sha256"`
	RecordedAt  *time.Time `json:"recordedAt,omitempty"`
}

// Note is a synced note.
type Note struct {
	ID            int64      `json:"id"`
	ClientID      string     `json:"clientId"`
	AuthorID      int64      `json:"authorId"`
	Author        string     `json:"author"`
	PlaceType     string     `json:"placeType,omitempty"`
	PlaceName     string     `json:"placeName,omitempty"`
	Lon           *float64   `json:"lon,omitempty"`
	Lat           *float64   `json:"lat,omitempty"`
	Address       string     `json:"address,omitempty"`
	Neighbourhood string     `json:"neighbourhood,omitempty"`
	GeocodeSource string     `json:"geocodeSource,omitempty"`
	GeocodeFlags  []string   `json:"geocodeFlags,omitempty"`
	CalledAt      *time.Time `json:"calledAt,omitempty"`
	ExaminedAt    *time.Time `json:"examinedAt,omitempty"`
	OccurredAt    *time.Time `json:"occurredAt,omitempty"`
	Team          []Member   `json:"team"`
	OtherOfficers string     `json:"otherOfficers,omitempty"`
	Notes         string     `json:"notes,omitempty"`
	WrittenAt     time.Time  `json:"writtenAt"`
	SyncedAt      time.Time  `json:"syncedAt"`
	CollectionID  *int64     `json:"collectionId,omitempty"` // set once it became a collection
	AudioCount    int        `json:"audioCount"`
	Audios        []Audio    `json:"audios,omitempty"` // on one note (Get, Sync), not in lists
}

func nullString(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }

func nullTime(t *time.Time) sql.NullTime {
	if t == nil || t.IsZero() {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}

func timePtr(t sql.NullTime) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}

func check(in *Input, now time.Time) (uuid.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(in.ClientID))
	if err != nil {
		return uuid.Nil, ValidationError("clientId must be a UUID made on the device")
	}
	in.PlaceType = strings.ToUpper(strings.TrimSpace(in.PlaceType))
	in.PlaceName, in.Address, in.Neighbourhood = strings.TrimSpace(in.PlaceName), strings.TrimSpace(in.Address), strings.TrimSpace(in.Neighbourhood)
	in.OtherOfficers, in.Notes = strings.TrimSpace(in.OtherOfficers), strings.TrimSpace(in.Notes)
	switch {
	case in.PlaceType != "" && !slices.Contains(placeTypes, in.PlaceType):
		return uuid.Nil, ValidationError("unknown place type " + in.PlaceType)
	case (in.Lon == nil) != (in.Lat == nil):
		return uuid.Nil, ValidationError("a point needs both lon and lat")
	case in.Lon != nil && (*in.Lon < -180 || *in.Lon > 180 || *in.Lat < -90 || *in.Lat > 90):
		return uuid.Nil, ValidationError("the point is not on the map")
	case in.WrittenAt.IsZero():
		return uuid.Nil, ValidationError("writtenAt is required")
	case in.WrittenAt.After(now.Add(24 * time.Hour)):
		// A device's clock may be off, but not by days.
		return uuid.Nil, ValidationError("writtenAt is in the future")
	}
	team := in.Team[:0:0]
	for _, member := range in.Team {
		if member > 0 && !slices.Contains(team, member) {
			team = append(team, member)
		}
	}
	in.Team = team
	return id, nil
}

// Sync keeps a note sent from a device: a new one, or the same one again with what it says now. A
// point with no address typed gets the nearest address the geocoder knows, when it knows one.
func (s Service) Sync(ctx context.Context, author int64, in Input, now time.Time) (Note, error) {
	clientID, err := check(&in, now)
	if err != nil {
		return Note{}, err
	}
	q := db.New(s.DB)
	if len(in.Team) > 0 {
		users, err := q.ListUsernamesByIDs(ctx, in.Team)
		if err != nil {
			return Note{}, err
		}
		if len(users) != len(in.Team) {
			return Note{}, ValidationError("a member of the team is not a user")
		}
	}
	params := db.UpsertSceneNoteParams{ClientID: clientID, AuthorID: author, PlaceType: nullString(in.PlaceType),
		PlaceName: nullString(in.PlaceName), Address: nullString(in.Address), Neighbourhood: nullString(in.Neighbourhood),
		CalledAt: nullTime(in.CalledAt), ExaminedAt: nullTime(in.ExaminedAt), OccurredAt: nullTime(in.OccurredAt),
		Team: in.Team, OtherOfficers: nullString(in.OtherOfficers), Notes: nullString(in.Notes), WrittenAt: in.WrittenAt}
	if in.Lon != nil {
		params.Lon, params.Lat = sql.NullFloat64{Float64: *in.Lon, Valid: true}, sql.NullFloat64{Float64: *in.Lat, Valid: true}
		// Found here, not at the scene: the device had no signal, and nothing typed there leaves
		// the network.
		if in.Address == "" && s.Geocoder != nil {
			if found, err := s.Geocoder.Reverse(ctx, *in.Lon, *in.Lat); err == nil {
				address := strings.Join(nonEmpty(found.Street, found.HouseNumber), ", ")
				if address == "" {
					address = found.DisplayName
				}
				params.Address = nullString(address)
				if in.Neighbourhood == "" {
					params.Neighbourhood = nullString(found.Neighbourhood)
				}
				params.GeocodeSource, params.GeocodeFlags = nullString(found.Source), found.Flags
			}
		}
	}
	row, err := q.UpsertSceneNote(ctx, params)
	if errors.Is(err, sql.ErrNoRows) {
		// The id exists and wasn't replaced: someone else's, or already a collection.
		existing, err := q.GetSceneNoteByClientID(ctx, clientID)
		if err != nil {
			return Note{}, err
		}
		if existing.AuthorID != author {
			return Note{}, ErrForbidden
		}
		// Synced again after it became a collection: it is kept as it was.
		return s.get(ctx, q, existing.ID)
	}
	if err != nil {
		return Note{}, fmt.Errorf("scenenotes: sync: %w", err)
	}
	return s.get(ctx, q, row.ID)
}

func nonEmpty(values ...string) []string {
	var out []string
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			out = append(out, v)
		}
	}
	return out
}

func (s Service) members(ctx context.Context, q *db.Queries, ids []int64) ([]Member, error) {
	out := []Member{}
	if len(ids) == 0 {
		return out, nil
	}
	users, err := q.ListUsernamesByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, u := range users {
		out = append(out, Member{ID: u.ID, Username: u.Username, Name: strings.TrimSpace(u.FirstName + " " + u.LastName)})
	}
	return out, nil
}

func (s Service) get(ctx context.Context, q *db.Queries, id int64) (Note, error) {
	r, err := q.GetSceneNote(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Note{}, ErrNotFound
	}
	if err != nil {
		return Note{}, err
	}
	n := Note{ID: r.ID, ClientID: r.ClientID.String(), AuthorID: r.AuthorID, Author: r.AuthorName, PlaceType: r.PlaceType.String,
		PlaceName: r.PlaceName.String, Address: r.Address.String, Neighbourhood: r.Neighbourhood.String,
		GeocodeSource: r.GeocodeSource.String, GeocodeFlags: r.GeocodeFlags, CalledAt: timePtr(r.CalledAt),
		ExaminedAt: timePtr(r.ExaminedAt), OccurredAt: timePtr(r.OccurredAt), OtherOfficers: r.OtherOfficers.String,
		Notes: r.Notes.String, WrittenAt: r.WrittenAt, SyncedAt: r.SyncedAt}
	if r.Lon.Valid {
		n.Lon, n.Lat = &r.Lon.Float64, &r.Lat.Float64
	}
	if r.CollectionID.Valid {
		n.CollectionID = &r.CollectionID.Int64
	}
	if n.Team, err = s.members(ctx, q, r.Team); err != nil {
		return Note{}, err
	}
	audios, err := q.ListSceneNoteAudios(ctx, id)
	if err != nil {
		return Note{}, err
	}
	n.Audios = []Audio{}
	for _, a := range audios {
		n.Audios = append(n.Audios, Audio{ID: a.ID, ClientID: a.ClientID.String(), Title: a.Title.String, ContentType: a.ContentType,
			SizeBytes: a.SizeBytes, SHA256: a.Sha256, RecordedAt: timePtr(a.RecordedAt)})
	}
	n.AudioCount = len(n.Audios)
	return n, nil
}

// reads reports whether user may read a note: its author, or a coordinator.
func reads(user auth.UserProfile, authorID int64) bool {
	return user.ID == authorID || user.Coordinates()
}

// Get returns one note with its audios, to its author or a coordinator.
func (s Service) Get(ctx context.Context, user auth.UserProfile, id int64) (Note, error) {
	n, err := s.get(ctx, db.New(s.DB), id)
	if err != nil {
		return Note{}, err
	}
	if !reads(user, n.AuthorID) {
		return Note{}, ErrForbidden
	}
	return n, nil
}

// List returns the user's notes, newest first, or everyone's for a coordinator who asks (everyone).
// Notes that already became a collection are left out unless converted.
func (s Service) List(ctx context.Context, user auth.UserProfile, everyone, converted bool) ([]Note, error) {
	q := db.New(s.DB)
	rows, err := q.ListSceneNotes(ctx, db.ListSceneNotesParams{Everyone: everyone && user.Coordinates(), AuthorID: user.ID, Converted: converted})
	if err != nil {
		return nil, err
	}
	out := make([]Note, 0, len(rows))
	for _, r := range rows {
		n := Note{ID: r.ID, ClientID: r.ClientID.String(), AuthorID: r.AuthorID, Author: r.AuthorName, PlaceType: r.PlaceType.String,
			PlaceName: r.PlaceName.String, Address: r.Address.String, Neighbourhood: r.Neighbourhood.String,
			GeocodeSource: r.GeocodeSource.String, GeocodeFlags: r.GeocodeFlags, CalledAt: timePtr(r.CalledAt),
			ExaminedAt: timePtr(r.ExaminedAt), OccurredAt: timePtr(r.OccurredAt), OtherOfficers: r.OtherOfficers.String,
			Notes: r.Notes.String, WrittenAt: r.WrittenAt, SyncedAt: r.SyncedAt, AudioCount: int(r.AudioCount)}
		if r.Lon.Valid {
			n.Lon, n.Lat = &r.Lon.Float64, &r.Lat.Float64
		}
		if r.CollectionID.Valid {
			n.CollectionID = &r.CollectionID.Int64
		}
		if n.Team, err = s.members(ctx, q, r.Team); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
}

// Discard removes a note that is not a collection yet: its author's, or any for a coordinator. Its
// audio files stay in storage, which does not delete.
func (s Service) Discard(ctx context.Context, user auth.UserProfile, id int64) error {
	q := db.New(s.DB)
	r, err := q.GetSceneNote(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if !reads(user, r.AuthorID) {
		return ErrForbidden
	}
	n, err := q.DeleteSceneNote(ctx, id)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrConverted
	}
	return nil
}

// AudioInput is an audio note as the device sends it.
type AudioInput struct {
	ClientID    string
	Title       string
	ContentType string
	RecordedAt  *time.Time
	Content     []byte
}

// AddAudio keeps an audio recorded with a note. The same audio again (its ClientID) is the one
// already kept.
func (s Service) AddAudio(ctx context.Context, user auth.UserProfile, noteID int64, in AudioInput) (Audio, error) {
	clientID, err := uuid.Parse(strings.TrimSpace(in.ClientID))
	if err != nil {
		return Audio{}, ValidationError("clientId must be a UUID made on the device")
	}
	contentType := strings.ToLower(strings.TrimSpace(strings.SplitN(in.ContentType, ";", 2)[0]))
	switch {
	case len(in.Content) == 0:
		return Audio{}, ValidationError("the audio is empty")
	case len(in.Content) > MaxAudioBytes:
		return Audio{}, ValidationError("the audio is too large")
	// A phone's recorder names its sound-only WebM and MP4 files either way.
	case !strings.HasPrefix(contentType, "audio/") && contentType != "video/webm" && contentType != "video/mp4":
		return Audio{}, ValidationError("the file is not audio")
	}
	q := db.New(s.DB)
	note, err := q.GetSceneNote(ctx, noteID)
	if errors.Is(err, sql.ErrNoRows) {
		return Audio{}, ErrNotFound
	}
	if err != nil {
		return Audio{}, err
	}
	if note.AuthorID != user.ID {
		return Audio{}, ErrForbidden
	}
	sum := sha256.Sum256(in.Content)
	hash := hex.EncodeToString(sum[:])
	title := strings.TrimSpace(in.Title)
	out := Audio{ClientID: clientID.String(), Title: title, ContentType: contentType, SizeBytes: int64(len(in.Content)),
		SHA256: hash, RecordedAt: in.RecordedAt}
	if existing, err := q.GetSceneNoteAudioByClientID(ctx, clientID); err == nil {
		if existing.SceneNoteID != noteID || existing.Sha256 != hash {
			return Audio{}, ValidationError("this audio id is already another recording")
		}
		out.ID = existing.ID
		return out, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return Audio{}, err
	}
	if note.CollectionID.Valid {
		return Audio{}, ErrConverted
	}
	ref, err := s.Store.Upload(ctx, fmt.Sprintf("scene-notes/%d/%s", noteID, clientID), in.Content, contentType)
	if err != nil {
		return Audio{}, fmt.Errorf("scenenotes: store audio: %w", err)
	}
	out.ID, err = q.InsertSceneNoteAudio(ctx, db.InsertSceneNoteAudioParams{SceneNoteID: noteID, ClientID: clientID,
		Title: nullString(title), ContentType: contentType, SizeBytes: out.SizeBytes, Sha256: hash, StorageRef: ref,
		RecordedAt: nullTime(in.RecordedAt)})
	if err != nil {
		// Two syncs of the same audio at once: the other one kept it.
		if existing, getErr := q.GetSceneNoteAudioByClientID(ctx, clientID); getErr == nil && existing.Sha256 == hash {
			out.ID = existing.ID
			return out, nil
		}
		return Audio{}, fmt.Errorf("scenenotes: audio: %w", err)
	}
	return out, nil
}

// AudioContent returns an audio note's bytes and type, to the note's author or a coordinator.
func (s Service) AudioContent(ctx context.Context, user auth.UserProfile, audioID int64) ([]byte, string, error) {
	a, err := db.New(s.DB).GetSceneNoteAudio(ctx, audioID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", err
	}
	if !reads(user, a.AuthorID) {
		return nil, "", ErrForbidden
	}
	content, err := s.Store.Download(ctx, a.StorageRef)
	return content, a.ContentType, err
}
