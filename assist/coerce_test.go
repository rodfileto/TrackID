package assist

import (
	"testing"
	"time"

	"github.com/rodfileto/trackid/occurrences"
	"github.com/rodfileto/trackid/requests"
)

func draftWith(fn func(*Draft)) Draft {
	d := Draft{Request: requests.RegisterInput{
		Input: requests.Input{
			Authority:         " 37ª Delegacia  ",
			DocumentReference: " Ofício nº 1/2026 ",
			ReceivedOn:        "2026-01-02",
			Questions:         []string{"a pergunta", "  "},
		},
		CaseType: "criminal",
		Cases: []requests.CaseInput{
			{Task: "facial_review", Description: "  revisão  "},
		},
	}}
	if fn != nil {
		fn(&d)
	}
	return d
}

func occurrenceWith(fn func(*occurrences.CreateInput)) *occurrences.CreateInput {
	o := &occurrences.CreateInput{
		Types:        []string{"LETHAL"},
		OccurredFrom: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		Status:       "registered",
		Description:  "  um fato  ",
		Place:        occurrences.PlaceInput{Lon: -43.1, Lat: -22.9, Precision: "exact"},
	}
	if fn != nil {
		fn(o)
	}
	return o
}

func TestCoerceNormalizes(t *testing.T) {
	d := draftWith(func(d *Draft) { d.Request.Occurrence = occurrenceWith(nil) })

	out := Coerce(d, []string{"LETHAL"})

	if out.Request.Input.Authority != "37ª Delegacia" {
		t.Errorf("authority not trimmed: %q", out.Request.Input.Authority)
	}
	if out.Request.CaseType != "CRIMINAL" {
		t.Errorf("caseType not uppercased: %q", out.Request.CaseType)
	}
	if len(out.Request.Cases) != 1 || out.Request.Cases[0].Task != "FACIAL_REVIEW" || out.Request.Cases[0].Description != "revisão" {
		t.Errorf("case not normalized: %+v", out.Request.Cases)
	}
	if len(out.Request.Questions) != 1 {
		t.Errorf("blank question not dropped: %+v", out.Request.Questions)
	}
	if out.Request.Occurrence.Status != "REGISTERED" || out.Request.Occurrence.Place.Precision != "EXACT" {
		t.Errorf("occurrence not normalized: %+v", out.Request.Occurrence)
	}
	if out.Request.Occurrence.Description != "um fato" {
		t.Errorf("occurrence description not trimmed: %q", out.Request.Occurrence.Description)
	}
}

func TestCoerceDropsInvalid(t *testing.T) {
	d := draftWith(func(d *Draft) {
		d.Request.CaseType = "invented"
		d.Request.Cases = []requests.CaseInput{{Task: "NOPE", Description: "x"}}
		d.Request.Occurrence = occurrenceWith(func(o *occurrences.CreateInput) {
			o.Types = []string{"LETHAL", "BOGUS"}
			o.Status = "weird"
		})
	})

	out := Coerce(d, []string{"LETHAL"})

	if out.Request.CaseType != "" {
		t.Errorf("invalid caseType kept: %q", out.Request.CaseType)
	}
	if len(out.Request.Cases) != 0 {
		t.Errorf("case with invalid task kept: %+v", out.Request.Cases)
	}
	if got := out.Request.Occurrence.Types; len(got) != 1 || got[0] != "LETHAL" {
		t.Errorf("invalid occurrence type kept: %v", got)
	}
	if out.Request.Occurrence.Status != "" {
		t.Errorf("invalid status kept: %q", out.Request.Occurrence.Status)
	}
	for _, want := range []string{"caseType", "cases[0].task", "occurrence.types", "occurrence.status"} {
		if !contains(out.Unsure, want) {
			t.Errorf("unsure missing %q: %v", want, out.Unsure)
		}
	}
}

func TestCoerceDropsMalformedDate(t *testing.T) {
	d := draftWith(func(d *Draft) { d.Request.Input.ReceivedOn = "02/01/2026" })

	out := Coerce(d, nil)

	if out.Request.Input.ReceivedOn != "" {
		t.Errorf("malformed date kept: %q", out.Request.Input.ReceivedOn)
	}
	if !contains(out.Unsure, "receivedOn") {
		t.Errorf("unsure missing receivedOn: %v", out.Unsure)
	}
}

func TestCoerceDropsEmptyOccurrence(t *testing.T) {
	d := draftWith(func(d *Draft) { d.Request.Occurrence = &occurrences.CreateInput{} })

	out := Coerce(d, nil)

	if out.Request.Occurrence != nil {
		t.Errorf("empty occurrence kept: %+v", out.Request.Occurrence)
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
