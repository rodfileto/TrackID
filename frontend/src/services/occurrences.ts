import { apiBaseUrl, apiFetch, requestApi } from "./api";
import { getToken } from "./auth";

/** A biometric case linked to an occurrence -- see api.OccurrenceCase. */
export interface OccurrenceCase {
  caseId: string;
  modality: string;
  caseType: string;
}

/** A map feature's properties -- see api.OccurrenceProperties. Codes (types,
 * group, status, precision, placeType) are untranslated. */
export interface OccurrenceProperties {
  id: number;
  /** "2026.0001" for an occurrence registered in TrackID. */
  trackidNumber?: string;
  /** The police agency's own report number, when known. */
  agency?: string;
  number?: string;
  /** Absent when unknown. */
  occurredFrom?: string;
  occurredTo?: string;
  registeredAt: string;
  attempted?: boolean;
  status: string;
  description: string;
  types: string[];
  group: string;
  caseType: string;
  locationId: string;
  precision: string;
  placeType?: string;
  placeName?: string;
  neighbourhood?: string;
  /** What the geocoder said about the point or the address: see GeocodeResult. */
  geocodeSource?: string;
  geocodeFlags?: string[];
  cases: OccurrenceCase[];
  /** On a person's or a cluster's map: which of its own cases this occurrence is of. */
  inSet?: string[];
  /** The best certainty of those links: confirmed, unreviewed (the matcher's) or uncertain. */
  certainty?: "confirmed" | "unreviewed" | "uncertain";
  /** The kinds of those cases; both when the occurrence fuses facial and fingerprint ones. */
  modalities?: string[];
  municipalityCode?: string;
}

export interface OccurrenceFeature {
  type: "Feature";
  geometry: { type: "Point"; coordinates: [number, number] };
  properties: OccurrenceProperties;
}

/** A GeoJSON FeatureCollection, read as is by the map. */
export interface OccurrenceMap {
  type: "FeatureCollection";
  features: OccurrenceFeature[];
  truncated: boolean;
}

export interface OccurrenceType {
  code: string;
  caseType: string;
  group: string;
}

function authHeaders(): Record<string, string> {
  const token = getToken();
  return token ? { Authorization: `Bearer ${token}` } : {};
}

/** Occurrences as GeoJSON points, optionally for one year and some groups. */
export async function getOccurrenceMap(
  params: { year?: number; groups?: string[] } = {},
): Promise<OccurrenceMap> {
  const query = new URLSearchParams();
  if (params.year) query.set("year", String(params.year));
  if (params.groups && params.groups.length > 0)
    query.set("group", params.groups.join(","));
  const qs = query.toString();
  return requestApi<OccurrenceMap>(`/occurrences/map${qs ? `?${qs}` : ""}`, {
    headers: authHeaders(),
  });
}

export async function listOccurrenceTypes(): Promise<OccurrenceType[]> {
  const { items } = await requestApi<{ items: OccurrenceType[] }>(
    "/occurrences/types",
    { headers: authHeaders() },
  );
  return items;
}

export async function listOccurrenceYears(): Promise<number[]> {
  const { years } = await requestApi<{ years: number[] }>(
    "/occurrences/years",
    { headers: authHeaders() },
  );
  return years;
}

/** An occurrence as a list or the case page shows it -- see api.OccurrenceSummary. */
export interface OccurrenceSummary {
  id: number;
  trackidNumber?: string;
  agency?: string;
  number?: string;
  inquiryNumber?: string;
  occurredFrom?: string;
  status: string;
  types: string[];
  placeName?: string;
  neighbourhood?: string;
  /** Where it happened: sent for a case's occurrence (getCaseOccurrence), not by searches. */
  address?: string;
  /** Sent when it has no place yet. */
  description?: string;
  precision?: string;
  lon?: number;
  lat?: number;
  geocodeSource?: string;
  geocodeFlags?: string[];
}

/** Where a new occurrence happened: an existing location, or a point. */
export interface PlaceInput {
  existingId?: number;
  lon?: number;
  lat?: number;
  precision?: string;
  name?: string;
  address?: string;
  neighbourhood?: string;
  placeType?: string;
  /** What the geocoder said about the point or the address, when it found them. */
  geocodeSource?: string;
  geocodeFlags?: string[];
}

export interface CreateOccurrenceInput {
  types: string[];
  attempted?: boolean;
  occurredFrom?: string;
  occurredTo?: string;
  status?: string;
  description: string;
  agency?: string;
  agencyNumber?: string;
  inquiryNumber?: string;
  offenderCount?: number;
  victimCount?: number;
  place: PlaceInput;
  /** A case to link in the same request (the case-creation flow). */
  caseId?: string;
}

export interface CreatedOccurrence {
  id: number;
  trackidNumber: string;
  year: number;
  locationId: number;
  caseId?: string;
}

/** Registers an occurrence; TrackID numbers it. */
export async function createOccurrence(
  input: CreateOccurrenceInput,
): Promise<CreatedOccurrence> {
  try {
    return await requestApi<CreatedOccurrence>("/occurrences", {
      method: "POST",
      headers: authHeaders(),
      body: JSON.stringify(input),
      // Never leave the form saving forever: no answer in 30 s is an error.
      signal: AbortSignal.timeout(30_000),
    });
  } catch (err) {
    if (err instanceof DOMException && err.name === "TimeoutError") {
      throw new Error("The server did not answer in time; check whether the occurrence was registered before trying again.", {
        cause: err,
      });
    }
    throw err;
  }
}

/** Occurrences whose TrackID, police or inquiry number contains q. */
export async function searchOccurrences(q: string): Promise<OccurrenceSummary[]> {
  const { items } = await requestApi<{ items: OccurrenceSummary[] }>(
    `/occurrences/search?q=${encodeURIComponent(q)}`,
    { headers: authHeaders() },
  );
  return items;
}

/** The occurrence a case is linked to, or null. */
export async function getCaseOccurrence(
  caseId: string,
): Promise<OccurrenceSummary | null> {
  const { occurrence } = await requestApi<{ occurrence: OccurrenceSummary | null }>(
    `/cases/${encodeURIComponent(caseId)}/occurrence`,
    { headers: authHeaders() },
  );
  return occurrence;
}

/** A case's nature (CRIMINAL or CIVIL) and its occurrence, if linked. */
export async function getCaseOccurrenceLink(
  caseId: string,
): Promise<{ caseType: string; occurrence: OccurrenceSummary | null }> {
  return requestApi<{ caseType: string; occurrence: OccurrenceSummary | null }>(
    `/cases/${encodeURIComponent(caseId)}/occurrence`,
    { headers: authHeaders() },
  );
}

/** Links a case to an occurrence, or unlinks it (null). */
export async function linkCaseOccurrence(
  caseId: string,
  occurrenceId: number | null,
): Promise<void> {
  const response = await apiFetch(
    `${apiBaseUrl}/cases/${encodeURIComponent(caseId)}/occurrence`,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json", ...authHeaders() },
      body: JSON.stringify({ occurrenceId }),
    },
  );
  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(body.error ?? "The request could not be completed");
  }
}

export interface NearbyLocation {
  id: number;
  name?: string;
  address?: string;
  neighbourhood?: string;
  placeType?: string;
  precision: string;
  lon: number;
  lat: number;
  distanceM: number;
  occurrenceCount: number;
}

/** Existing locations near a point, for reuse. */
export async function nearbyLocations(
  lon: number,
  lat: number,
  radius = 50,
): Promise<NearbyLocation[]> {
  const { items } = await requestApi<{ items: NearbyLocation[] }>(
    `/locations/nearby?lon=${lon}&lat=${lat}&radius=${radius}`,
    { headers: authHeaders() },
  );
  return items;
}

/** A geocoder result -- see geocode.Result. */
export interface GeocodeResult {
  lon: number;
  lat: number;
  precision: string;
  displayName: string;
  street?: string;
  houseNumber?: string;
  neighbourhood?: string;
  city?: string;
  /** Where the point came from: "cnefe2022:…" (IBGE's census), "osm:…", "ibge:…" (a municipality). */
  source?: string;
  /** Reasons to check the point (geocode.Flag*), e.g. NEAREST_NUMBER. */
  flags?: string[];
}

/**
 * Geocodes an address. Called on an explicit search, never while typing. near
 * ([lon, lat], the map's centre) is the municipality an address naming none is
 * looked up in.
 */
export async function geocodeSearch(q: string, near?: [number, number]): Promise<GeocodeResult[]> {
  const nearParam = near ? `&near=${near[0]},${near[1]}` : "";
  const { items } = await requestApi<{ items: GeocodeResult[] }>(
    `/geocode/search?q=${encodeURIComponent(q)}${nearParam}`,
    { headers: authHeaders() },
  );
  return items;
}

/** The address at a point, or null. */
export async function geocodeReverse(
  lon: number,
  lat: number,
): Promise<GeocodeResult | null> {
  const { item } = await requestApi<{ item: GeocodeResult | null }>(
    `/geocode/reverse?lon=${lon}&lat=${lat}`,
    { headers: authHeaders() },
  );
  return item;
}

/** Which links a presence map follows: only those examiners confirmed, or every one but the excluded ones. */
export type PresenceLinks = "reviewed" | "all";

/** Where a person or a cluster appears: the occurrences of the cases it has traces in, with what could not be drawn. */
export interface PresenceMap extends OccurrenceMap {
  links: PresenceLinks;
  /** Of a cluster: AUTOMATIC (the matcher's) or REVIEWED (examiners'). */
  basis?: string;
  /** Cases it has traces in, and how many of them are facial and fingerprint. */
  cases: number;
  facial: number;
  fingerprint: number;
  /** Of them, with an occurrence. */
  withOccurrence: number;
  /** Distinct occurrences those cases have. */
  occurrences: number;
  /** Of those, with no place to draw. */
  unplaced: number;
  /** Occurrences holding both a facial and a fingerprint case of the set. */
  fused: number;
}

/** An area where its occurrences gather (presence.Area). */
export interface PresenceArea {
  rank: number;
  main: boolean;
  pointIds: number[];
  centre: [number, number];
  radiusM: number;
  hull: [number, number][];
  first?: string;
  last?: string;
  types: { type: string; count: number }[];
  reviewed: number;
  share: number;
  baseInside: number;
  /** Its share of the person's occurrences over its footprint's share of all of them; absent when the base is too small. */
  quotient?: number;
  fused: number;
}

export interface PresenceAreas {
  links: PresenceLinks;
  basis?: string;
  epsM: number;
  minPts: number;
  mapped: number;
  areas: PresenceArea[];
  noise: number[];
  municipalities: { code: string; name: string; uf: string; count: number }[];
  /** Fewer mapped occurrences than it takes to single out a main area. */
  tooFew: boolean;
}

export async function getPersonOccurrences(personId: string, links: PresenceLinks = "all"): Promise<PresenceMap> {
  return requestApi<PresenceMap>(`/persons/${encodeURIComponent(personId)}/occurrences?links=${links}`, { headers: authHeaders() });
}

export async function getClusterOccurrences(clusterId: number, links: PresenceLinks = "all"): Promise<PresenceMap> {
  return requestApi<PresenceMap>(`/clusters/${clusterId}/occurrences?links=${links}`, { headers: authHeaders() });
}

export async function getPersonPresenceAreas(personId: string, links: PresenceLinks, epsM: number): Promise<PresenceAreas> {
  return requestApi<PresenceAreas>(`/persons/${encodeURIComponent(personId)}/presence-areas?links=${links}&eps=${epsM}`, { headers: authHeaders() });
}

export async function getClusterPresenceAreas(clusterId: number, links: PresenceLinks, epsM: number): Promise<PresenceAreas> {
  return requestApi<PresenceAreas>(`/clusters/${clusterId}/presence-areas?links=${links}&eps=${epsM}`, { headers: authHeaders() });
}
