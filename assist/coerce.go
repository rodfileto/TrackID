package assist

import (
	"slices"
	"strings"

	"github.com/rodfileto/trackid/analyses"
	"github.com/rodfileto/trackid/occurrences"
	"github.com/rodfileto/trackid/requests"
)

// The enums Coerce checks against. These are the same lists the model is told to use (see
// openai.go's schema), and the same ones the real registration path enforces:
//   - case types, statuses, place types and precisions mirror cases.CreateWith and
//     occurrences.CreateWith (occurrences/create.go keeps its lists private);
//   - tasks are analyses.Type values, checked through its Valid/Offered methods rather than
//     duplicated here;
//   - occurrence types are not static: they come from the occurrence_types table, passed in.

const (
	caseTypeCriminal = "CRIMINAL"
	caseTypeCivil    = "CIVIL"
)

var (
	occurrenceStatuses = []string{"REGISTERED", "UNDER_INVESTIGATION", "INQUIRY_OPENED", "CLOSED"}
	placeTypes         = []string{"PUBLIC_ROAD", "RESIDENCE", "COMMERCE", "BANK", "GOVERNMENT_OFFICE", "TRANSPORT", "RURAL", "OTHER"}
	placePrecisions    = []string{"EXACT", "GEOCODED", "STREET", "NEIGHBOURHOOD", "MUNICIPALITY"}
)

// DefaultOccurrenceTypes is the occurrence_types seed (db/migrations/003_occurrences.sql), for
// contexts without a database: the manual-testing assist lab, which drafts without touching the
// case tables. The worker and the review flow fetch the same list live from the table; this copy
// exists only so a draft can be proposed and checked where there is no database at all.
var DefaultOccurrenceTypes = []string{
	"HOMICIDE", "FEMICIDE", "ROBBERY_HOMICIDE", "BODILY_INJURY_DEATH", "POLICE_INTERVENTION_DEATH", "NEGLIGENT_HOMICIDE",
	"ASSAULT", "NEGLIGENT_BODILY_INJURY", "RAPE", "KIDNAPPING", "EXPRESS_KIDNAPPING", "EXTORTION", "THREAT",
	"STREET_ROBBERY", "PHONE_ROBBERY", "PUBLIC_TRANSPORT_ROBBERY", "VEHICLE_ROBBERY", "CARGO_ROBBERY", "COMMERCIAL_ROBBERY",
	"HOME_ROBBERY", "BANK_ROBBERY", "ATM_ROBBERY", "FORCED_WITHDRAWAL_ROBBERY", "POST_WITHDRAWAL_ROBBERY", "BICYCLE_ROBBERY", "OTHER_ROBBERY",
	"VEHICLE_THEFT", "STREET_THEFT", "PUBLIC_TRANSPORT_THEFT", "PHONE_THEFT", "BICYCLE_THEFT", "OTHER_THEFT",
	"FRAUD", "IDENTITY_FRAUD",
	"DRUG_POSSESSION", "DRUG_TRAFFICKING",
	"MISSING_PERSON", "BODY_FOUND", "SKELETAL_REMAINS_FOUND", "UNIDENTIFIED_LIVING_PERSON", "DISASTER_VICTIM",
}

// Coerce normalizes a model's draft and strips what the registration path would refuse, so the
// review screen never shows a value that cannot be submitted. It is lenient: a missing or
// undecided field is left empty, and only a value the model produced that is not one of the
// allowed codes is dropped. Every dropped value is named in Draft.Unsure, so the coordinator sees
// exactly what the model got wrong. occurrenceTypes are the codes of the occurrence_types table
// (the model's choices are checked against them).
func Coerce(d Draft, occurrenceTypes []string) Draft {
	out := d
	out.Request.Input.Authority = strings.TrimSpace(d.Request.Input.Authority)
	out.Request.Input.DocumentReference = strings.TrimSpace(d.Request.Input.DocumentReference)
	out.Request.Input.DocumentDate = coerceDate(d.Request.Input.DocumentDate, &out, "documentDate")
	out.Request.Input.ReceivedOn = coerceDate(d.Request.Input.ReceivedOn, &out, "receivedOn")
	out.Request.Input.DueOn = coerceDate(d.Request.Input.DueOn, &out, "dueOn")
	out.Request.Input.CitedReference = strings.TrimSpace(d.Request.Input.CitedReference)
	out.Request.Input.CitedInquiry = strings.TrimSpace(d.Request.Input.CitedInquiry)
	out.Request.Input.Notes = strings.TrimSpace(d.Request.Input.Notes)
	out.Request.Questions = nonEmpty(d.Request.Questions)

	out.Request.CaseType = coerceEnum(d.Request.CaseType, []string{caseTypeCriminal, caseTypeCivil}, &out, "caseType")

	var cases []requests.CaseInput
	for i, c := range d.Request.Cases {
		task := analyses.Type(strings.ToUpper(strings.TrimSpace(c.Task)))
		if !task.Valid() || !task.Offered() {
			addUnsure(&out, "cases["+itoa(i)+"].task")
			continue
		}
		cases = append(cases, requests.CaseInput{Task: string(task), Description: strings.TrimSpace(c.Description)})
	}
	out.Request.Cases = cases

	if occ := d.Request.Occurrence; occ != nil {
		if occurrenceEmpty(occ) {
			out.Request.Occurrence = nil
		} else {
			out.Request.Occurrence = coerceOccurrence(occ, occurrenceTypes, &out)
		}
	}
	return out
}

func coerceOccurrence(occ *occurrences.CreateInput, occurrenceTypes []string, out *Draft) *occurrences.CreateInput {
	o := *occ
	o.Types = nil
	for _, t := range occ.Types {
		t = strings.ToUpper(strings.TrimSpace(t))
		if slices.Contains(occurrenceTypes, t) {
			o.Types = append(o.Types, t)
		}
	}
	if len(occ.Types) != len(o.Types) {
		addUnsure(out, "occurrence.types")
	}
	o.Status = coerceEnum(occ.Status, occurrenceStatuses, out, "occurrence.status")
	o.Place.Precision = coerceEnum(occ.Place.Precision, placePrecisions, out, "occurrence.place.precision")
	o.Place.PlaceType = coerceEnum(occ.Place.PlaceType, placeTypes, out, "occurrence.place.placeType")
	o.Place.Name = strings.TrimSpace(occ.Place.Name)
	o.Place.Address = strings.TrimSpace(occ.Place.Address)
	o.Place.Neighbourhood = strings.TrimSpace(occ.Place.Neighbourhood)
	o.Description = strings.TrimSpace(occ.Description)
	o.Agency = strings.TrimSpace(occ.Agency)
	o.AgencyNumber = strings.TrimSpace(occ.AgencyNumber)
	o.InquiryNumber = strings.TrimSpace(occ.InquiryNumber)
	return &o
}

// occurrenceEmpty reports whether the model proposed nothing at all: no types, no date, no
// description, no identity and no place. Such an occurrence is dropped rather than sent for
// review as an empty form.
func occurrenceEmpty(o *occurrences.CreateInput) bool {
	return len(o.Types) == 0 && o.OccurredFrom.IsZero() && o.Description == "" &&
		o.Agency == "" && o.AgencyNumber == "" && o.InquiryNumber == "" &&
		o.Place.Lon == 0 && o.Place.Lat == 0
}

// coerceEnum uppercases and trims a code; when the model produced something not in allowed, it
// drops it to "" and records the field as unsure.
func coerceEnum(v string, allowed []string, out *Draft, field string) string {
	v = strings.ToUpper(strings.TrimSpace(v))
	if v == "" {
		return ""
	}
	if !slices.Contains(allowed, v) {
		addUnsure(out, field)
		return ""
	}
	return v
}

// coerceDate trims a YYYY-MM-DD string and drops one that is not in that shape.
func coerceDate(v string, out *Draft, field string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if parseDate(v).IsZero() {
		addUnsure(out, field)
		return ""
	}
	return v
}

func addUnsure(out *Draft, field string) {
	if slices.Contains(out.Unsure, field) {
		return
	}
	out.Unsure = append(out.Unsure, field)
}

func nonEmpty(in []string) []string {
	var out []string
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [8]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(b[pos:])
}
