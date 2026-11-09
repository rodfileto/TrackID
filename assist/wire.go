package assist

import (
	"strings"
	"time"

	"github.com/rodfileto/trackid/occurrences"
	"github.com/rodfileto/trackid/requests"
)

// dateLayout is how dates cross the model boundary: as YYYY-MM-DD strings in requests.Input (its
// own layout) and as time.Time in occurrences.CreateInput.
const dateLayout = "2006-01-02"

// draftJSON is the shape a model is asked to produce: a request plus what it opens. Dates are
// plain strings (the model writes "2006-01-02", never a timestamp), enums are codes, and fields
// it did not find are left empty. It mirrors requests.RegisterInput but with no server-side-only
// fields: the model proposes new cases, so there is no caseId to join, and no occurrenceId --
// those resolve inside requests.Register, after the coordinator has reviewed.
type draftJSON struct {
	Authority         string   `json:"authority"`
	DocumentReference string   `json:"documentReference"`
	DocumentDate      string   `json:"documentDate"`
	ReceivedOn        string   `json:"receivedOn"`
	DueOn             string   `json:"dueOn"`
	CitedReference    string   `json:"citedReference"`
	CitedInquiry      string   `json:"citedInquiryNumber"`
	Notes             string   `json:"notes"`
	Questions         []string `json:"questions"`

	CaseType   string           `json:"caseType"`
	Occurrence *draftOccurrence `json:"occurrence"`
	Cases      []draftCase      `json:"cases"`

	Unsure    []string `json:"unsure"`
	Reasoning string   `json:"reasoning"`
}

// draftCase is one case the model proposes the request opens: the task it asks (an analysis type,
// whose modality is the case's) and a description.
type draftCase struct {
	Task        string `json:"task"`
	Description string `json:"description"`
}

// draftOccurrence is the occurrence the model proposes to create. It has no place id and no case
// id: the model places the occurrence at a point on the map, and requests.Register links the new
// case to it.
type draftOccurrence struct {
	Types         []string   `json:"types"`
	Attempted     *bool      `json:"attempted"`
	OccurredFrom  string     `json:"occurredFrom"`
	OccurredTo    string     `json:"occurredTo"`
	ReportedAt    string     `json:"reportedAt"`
	Status        string     `json:"status"`
	Description   string     `json:"description"`
	Agency        string     `json:"agency"`
	AgencyNumber  string     `json:"agencyNumber"`
	InquiryNumber string     `json:"inquiryNumber"`
	OffenderCount *int16     `json:"offenderCount"`
	VictimCount   *int16     `json:"victimCount"`
	Place         draftPlace `json:"place"`
}

type draftPlace struct {
	Lon           float64 `json:"lon"`
	Lat           float64 `json:"lat"`
	Precision     string  `json:"precision"`
	Name          string  `json:"name"`
	Address       string  `json:"address"`
	Neighbourhood string  `json:"neighbourhood"`
	PlaceType     string  `json:"placeType"`
}

// register converts the model's wire shape into a Draft, best-effort: an unparseable date becomes
// the zero time (Coerce records it as unsure), an empty occurrence object becomes nil. Coerce is
// applied separately, by BuildDraft, so this stays a mechanical translation.
func (d draftJSON) register() Draft {
	req := requests.RegisterInput{
		Input: requests.Input{
			Authority:         d.Authority,
			DocumentReference: d.DocumentReference,
			DocumentDate:      d.DocumentDate,
			ReceivedOn:        d.ReceivedOn,
			DueOn:             d.DueOn,
			CitedReference:    d.CitedReference,
			CitedInquiry:      d.CitedInquiry,
			Notes:             d.Notes,
			Questions:         d.Questions,
		},
		CaseType: d.CaseType,
		Cases:    make([]requests.CaseInput, 0, len(d.Cases)),
	}
	for _, c := range d.Cases {
		req.Cases = append(req.Cases, requests.CaseInput{Task: c.Task, Description: c.Description})
	}
	if d.Occurrence != nil {
		req.Occurrence = d.Occurrence.createInput()
	}
	return Draft{Request: req, Unsure: d.Unsure, Reasoning: d.Reasoning}
}

func (o *draftOccurrence) createInput() *occurrences.CreateInput {
	in := &occurrences.CreateInput{
		Types:         o.Types,
		Attempted:     o.Attempted,
		OccurredFrom:  parseDate(o.OccurredFrom),
		Status:        o.Status,
		Description:   o.Description,
		Agency:        o.Agency,
		AgencyNumber:  o.AgencyNumber,
		InquiryNumber: o.InquiryNumber,
		OffenderCount: o.OffenderCount,
		VictimCount:   o.VictimCount,
		Place: occurrences.PlaceInput{
			Lon:           o.Place.Lon,
			Lat:           o.Place.Lat,
			Precision:     o.Place.Precision,
			Name:          o.Place.Name,
			Address:       o.Place.Address,
			Neighbourhood: o.Place.Neighbourhood,
			PlaceType:     o.Place.PlaceType,
		},
	}
	if t := parseDate(o.OccurredTo); !t.IsZero() {
		in.OccurredTo = &t
	}
	if t := parseDate(o.ReportedAt); !t.IsZero() {
		in.ReportedAt = &t
	}
	return in
}

func parseDate(s string) time.Time {
	s = strings.TrimSpace(s)
	if t, err := time.Parse(dateLayout, s); err == nil {
		return t
	}
	return time.Time{}
}
