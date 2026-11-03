package assist

import (
	"testing"
)

func TestDraftJSONRegister(t *testing.T) {
	d := draftJSON{
		Authority:         "37ª DP",
		DocumentReference: "Ofício 1",
		DocumentDate:      "2026-01-02",
		ReceivedOn:        "2026-01-03",
		DueOn:             "2026-02-03",
		CitedReference:    "123-45",
		CitedInquiry:      "IPL-9",
		Questions:         []string{"q1"},
		CaseType:          "CRIMINAL",
		Occurrence: &draftOccurrence{
			Types:        []string{"LETHAL"},
			OccurredFrom: "2026-01-01",
			OccurredTo:   "2026-01-02",
			ReportedAt:   "2026-01-03",
			Place:        draftPlace{Lon: -43.1, Lat: -22.9, Precision: "EXACT"},
		},
		Cases: []draftCase{{Task: "FACIAL_REVIEW", Description: "revisão"}},
	}

	got := d.register()

	if got.Request.Input.Authority != "37ª DP" {
		t.Errorf("authority: %q", got.Request.Input.Authority)
	}
	if got.Request.CaseType != "CRIMINAL" {
		t.Errorf("caseType: %q", got.Request.CaseType)
	}
	if len(got.Request.Cases) != 1 || got.Request.Cases[0].Task != "FACIAL_REVIEW" {
		t.Errorf("cases: %+v", got.Request.Cases)
	}
	o := got.Request.Occurrence
	if o == nil {
		t.Fatal("occurrence nil")
	}
	if o.OccurredFrom.IsZero() {
		t.Error("occurredFrom zero")
	}
	if o.OccurredTo == nil || o.OccurredTo.IsZero() {
		t.Error("occurredTo not parsed")
	}
	if o.ReportedAt == nil || o.ReportedAt.IsZero() {
		t.Error("reportedAt not parsed")
	}
	if o.Place.Lon != -43.1 || o.Place.Lat != -22.9 {
		t.Errorf("place: %+v", o.Place)
	}
}

func TestDraftJSONRegisterUnparseableDate(t *testing.T) {
	d := draftJSON{Occurrence: &draftOccurrence{OccurredFrom: "não é data"}}

	got := d.register()

	if o := got.Request.Occurrence; o == nil || !o.OccurredFrom.IsZero() {
		t.Errorf("unparseable date should be zero time: %+v", got.Request.Occurrence)
	}
	if got.Request.Occurrence.OccurredTo != nil || got.Request.Occurrence.ReportedAt != nil {
		t.Errorf("unset pointers should stay nil: %+v", got.Request.Occurrence)
	}
}
