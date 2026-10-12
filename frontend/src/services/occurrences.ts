import { requestApi } from "./api";
import { getToken } from "./auth";

/** A biometric case linked to an occurrence -- see api.OccurrenceCase. */
export interface OccurrenceCase {
  caseId: string;
  modality: string;
  caseType: string;
}

/** A map feature's properties -- see api.OccurrenceProperties. Codes (types,
 * group, status, precision, placeType, modusOperandi) are untranslated. */
export interface OccurrenceProperties {
  id: number;
  agency: string;
  number: string;
  occurredFrom: string;
  occurredTo?: string;
  registeredAt: string;
  attempted?: boolean;
  status: string;
  modusOperandi: string[];
  description: string;
  types: string[];
  group: string;
  caseType: string;
  locationId: string;
  precision: string;
  placeType?: string;
  placeName?: string;
  neighbourhood?: string;
  cases: OccurrenceCase[];
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
