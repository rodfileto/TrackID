import type { CreateOccurrenceInput, OccurrenceType } from "../../services/occurrences";

/** An occurrence being filled in, as the form holds it. */
export interface OccurrenceDraft {
  chosen: string[];
  occurredFrom: string; // datetime-local
  foundLater: boolean;
  occurredTo: string;
  attempted: boolean;
  offenderCount: string;
  victimCount: string;
  status: string;
  agency: string;
  agencyNumber: string;
  inquiryNumber: string;
  description: string;
  place: CreateOccurrenceInput["place"];
}

export const emptyOccurrence: OccurrenceDraft = {
  chosen: [], occurredFrom: "", foundLater: false, occurredTo: "", attempted: false, offenderCount: "", victimCount: "",
  status: "REGISTERED", agency: "", agencyNumber: "", inquiryNumber: "", description: "", place: {},
};

/** A datetime-local value (the user's local time) as an ISO instant. */
function toISO(local: string): string | undefined {
  return local ? new Date(local).toISOString() : undefined;
}

function optionalInt(v: string): number | undefined {
  return v === "" ? undefined : Number(v);
}

/** The draft's nature: its first type's, or the given one before any type is chosen. */
export function occurrenceNature(d: OccurrenceDraft, types: OccurrenceType[], fallback?: string): string | undefined {
  const first = types.find((x) => x.code === d.chosen[0]);
  return first ? first.caseType : fallback;
}

/** The draft as the API takes it. */
export function occurrenceInput(d: OccurrenceDraft, types: OccurrenceType[]): CreateOccurrenceInput {
  const civil = occurrenceNature(d, types) === "CIVIL";
  return {
    types: d.chosen,
    attempted: civil ? undefined : d.attempted || undefined,
    occurredFrom: toISO(d.occurredFrom) ?? "",
    occurredTo: d.foundLater ? toISO(d.occurredTo) : undefined,
    status: d.status,
    description: d.description,
    agency: d.agency || undefined,
    agencyNumber: d.agencyNumber || undefined,
    inquiryNumber: d.inquiryNumber || undefined,
    offenderCount: civil ? undefined : optionalInt(d.offenderCount),
    victimCount: optionalInt(d.victimCount),
    place: d.place,
  };
}

/** Whether the draft has what an occurrence requires: a type, when, where and a description. */
export function occurrenceReady(d: OccurrenceDraft): boolean {
  const hasPlace = d.place.existingId !== undefined || (d.place.lon !== undefined && d.place.lat !== undefined);
  return d.chosen.length > 0 && d.occurredFrom !== "" && d.description.trim() !== "" && hasPlace;
}
