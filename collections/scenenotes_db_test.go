package collections_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/rodfileto/trackid/collections"
	"github.com/rodfileto/trackid/geocode"
	"github.com/rodfileto/trackid/scenenotes"
)

// memStore keeps files in memory.
type memStore map[string][]byte

func (m memStore) Upload(_ context.Context, key string, content []byte, _ string) (string, error) {
	m["mem://"+key] = content
	return "mem://" + key, nil
}

func (m memStore) Download(_ context.Context, ref string) ([]byte, error) {
	content, ok := m[ref]
	if !ok {
		return nil, fmt.Errorf("no object %s", ref)
	}
	return content, nil
}

// oneAddress answers every point with the same census address.
type oneAddress struct{}

func (oneAddress) Search(context.Context, geocode.Query) ([]geocode.Result, error) {
	return nil, geocode.ErrNotFound
}

func (oneAddress) Reverse(context.Context, float64, float64) (geocode.Result, error) {
	return geocode.Result{Street: "QR 308 CONJUNTO 13 LOTE 8", Neighbourhood: "SAMAMBAIA", Source: "cnefe2022:5300108"}, nil
}

// An examiner writes a note at a scene with no signal, and the device syncs it later, more than
// once: it stays one note, its point gets an address on the server, and its audio is kept once. It
// then becomes a scene collection, which takes the call-out time, the team and the audio; after
// that the note is no longer a draft.
func TestFieldNoteBecomesACollection(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	store := memStore{}
	notes := scenenotes.Service{DB: f.db, Store: store, Geocoder: oneAddress{}}
	lon, lat := -48.0746, -15.8773
	called, examined := f.now.Add(-3*time.Hour), f.now.Add(-2*time.Hour)
	in := scenenotes.Input{ClientID: "0b0e5d0e-7c0a-4c63-9f1e-0d5f6f1c2a01", PlaceType: "residence", PlaceName: "Casa dos fundos",
		Lon: &lon, Lat: &lat, CalledAt: &called, ExaminedAt: &examined, Team: []int64{f.coordinator.ID, f.coordinator.ID},
		OtherOfficers: "APF Fulano", Notes: "Janela arrombada", WrittenAt: examined}

	first, err := notes.Sync(ctx, f.collector.ID, in, f.now)
	if err != nil {
		t.Fatal(err)
	}
	if first.Address != "QR 308 CONJUNTO 13 LOTE 8" || first.Neighbourhood != "SAMAMBAIA" || first.GeocodeSource != "cnefe2022:5300108" ||
		first.PlaceType != "RESIDENCE" || len(first.Team) != 1 {
		t.Errorf("synced note: %+v", first)
	}
	// Again, with more written: the same note.
	in.Notes = "Janela arrombada; fragmentos no vidro"
	again, err := notes.Sync(ctx, f.collector.ID, in, f.now)
	if err != nil || again.ID != first.ID || again.Notes != in.Notes {
		t.Errorf("second sync: %+v %v", again, err)
	}
	// The same id from someone else's device is not theirs to write.
	if _, err := notes.Sync(ctx, f.coordinator.ID, in, f.now); !errors.Is(err, scenenotes.ErrForbidden) {
		t.Errorf("someone else's id: %v", err)
	}
	for name, bad := range map[string]func(*scenenotes.Input){
		"no uuid":       func(i *scenenotes.Input) { i.ClientID = "1" },
		"half a point":  func(i *scenenotes.Input) { i.Lat = nil },
		"unknown place": func(i *scenenotes.Input) { i.PlaceType = "CASTLE" },
		"unknown user":  func(i *scenenotes.Input) { i.Team = []int64{999999} },
		"no time":       func(i *scenenotes.Input) { i.WrittenAt = time.Time{} },
	} {
		wrong := in
		wrong.ClientID = "0b0e5d0e-7c0a-4c63-9f1e-0d5f6f1c2a09"
		bad(&wrong)
		var validation scenenotes.ValidationError
		if _, err := notes.Sync(ctx, f.collector.ID, wrong, f.now); !errors.As(err, &validation) {
			t.Errorf("%s: %v", name, err)
		}
	}

	audio := scenenotes.AudioInput{ClientID: "0b0e5d0e-7c0a-4c63-9f1e-0d5f6f1c2a02", Title: "Entrada", ContentType: "audio/webm;codecs=opus",
		RecordedAt: &examined, Content: []byte("not really opus")}
	kept, err := notes.AddAudio(ctx, f.collector, first.ID, audio)
	if err != nil {
		t.Fatal(err)
	}
	if once, err := notes.AddAudio(ctx, f.collector, first.ID, audio); err != nil || once.ID != kept.ID || len(store) != 1 {
		t.Errorf("the same audio again: %+v %v, %d stored", once, err, len(store))
	}
	other := audio
	other.Content = []byte("another recording")
	var validation scenenotes.ValidationError
	if _, err := notes.AddAudio(ctx, f.collector, first.ID, other); !errors.As(err, &validation) {
		t.Errorf("another recording under the same id: %v", err)
	}
	if _, err := notes.AddAudio(ctx, f.coordinator, first.ID, other); !errors.Is(err, scenenotes.ErrForbidden) {
		t.Errorf("an audio on someone else's note: %v", err)
	}
	if content, contentType, err := notes.AudioContent(ctx, f.coordinator, kept.ID); err != nil || string(content) != "not really opus" || contentType != "audio/webm" {
		t.Errorf("a coordinator hears it: %q %q %v", content, contentType, err)
	}

	// Its author lists it; a coordinator lists everyone's.
	if mine, err := notes.List(ctx, f.collector, false, false); err != nil || len(mine) != 1 || mine[0].AudioCount != 1 {
		t.Errorf("the author's notes: %+v %v", mine, err)
	}
	if theirs, _ := notes.List(ctx, f.coordinator, false, false); len(theirs) != 0 {
		t.Errorf("a coordinator's own notes: %+v", theirs)
	}
	if all, _ := notes.List(ctx, f.coordinator, true, false); len(all) != 1 {
		t.Errorf("everyone's, for a coordinator: %+v", all)
	}

	// Called out after collecting makes no sense.
	late := f.now
	if _, err := collections.Register(ctx, f.db, collections.RegisterInput{OccurrenceID: nil, CollectedBy: f.collector.ID,
		CollectedFrom: examined, CalledAt: &late}, f.collector.ID, f.now); !isValidation(err) {
		t.Errorf("called out after collecting: %v", err)
	}
	r := f.scene(t, collections.RegisterInput{CollectedFrom: examined, CalledAt: &called, OtherOfficers: " APF Fulano ",
		Team: []int64{f.coordinator.ID, f.collector.ID}, SceneNoteID: first.ID, NothingCollected: true})
	origins, err := collections.List(ctx, f.db, r.CaseID)
	if err != nil || len(origins.Collections) != 1 {
		t.Fatal(origins, err)
	}
	col := origins.Collections[0]
	if col.CalledAt == nil || !col.CalledAt.Equal(called) || col.OtherOfficers != "APF Fulano" ||
		len(col.Team) != 1 || col.Team[0].ID != f.coordinator.ID || // the collector is not their own team
		len(col.AudioNotes) != 1 || col.AudioNotes[0].Filename != "Entrada" || col.AudioNotes[0].ContentType != "audio/webm" {
		t.Errorf("collection from a note: %+v", col)
	}

	// The note is a collection now: no longer listed as a draft, not replaced, discarded or used again.
	if drafts, _ := notes.List(ctx, f.collector, false, false); len(drafts) != 0 {
		t.Errorf("drafts after converting: %+v", drafts)
	}
	in.Notes = "written after"
	if after, err := notes.Sync(ctx, f.collector.ID, in, f.now); err != nil || after.CollectionID == nil || after.Notes == "written after" {
		t.Errorf("synced after converting: %+v %v", after, err)
	}
	if err := notes.Discard(ctx, f.collector, first.ID); !errors.Is(err, scenenotes.ErrConverted) {
		t.Errorf("discarding a converted note: %v", err)
	}
	if _, err := collections.Register(ctx, f.db, collections.RegisterInput{OccurrenceID: &r.OccurrenceID, CollectedBy: f.collector.ID,
		CollectedFrom: examined, SceneNoteID: first.ID}, f.collector.ID, f.now); !isValidation(err) {
		t.Errorf("one note, two collections: %v", err)
	}

	// A draft never converted is discarded by its author.
	in.ClientID = "0b0e5d0e-7c0a-4c63-9f1e-0d5f6f1c2a03"
	draft, err := notes.Sync(ctx, f.collector.ID, in, f.now)
	if err != nil {
		t.Fatal(err)
	}
	if err := notes.Discard(ctx, f.collector, draft.ID); err != nil {
		t.Errorf("discard: %v", err)
	}
	if _, err := notes.Get(ctx, f.collector, draft.ID); !errors.Is(err, scenenotes.ErrNotFound) {
		t.Errorf("a discarded note: %v", err)
	}
}
