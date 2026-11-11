import { apiBaseUrl, apiFetch, requestApi } from "./api";
import { getToken } from "./auth";
import type { CreateOccurrenceInput } from "./occurrences";

// Where a case's material came from (docs/scene-collection-plan.md): received with a request, or
// collected at a scene an examiner examined on call; and the physical items (vestiges) with their seals.

export type ReportMode = "SEPARATE" | "TOGETHER";

export interface Seal {
  id: number;
  number: string;
  appliedAt: string;
  /** Empty when applied outside, by the agency that sent the item. */
  appliedBy?: string;
  brokenAt?: string;
  brokenBy?: string;
}

export interface Vestige {
  id: number;
  sequence: number;
  label: string;
  description: string;
  requestId?: number;
  collectionId?: number;
  seals: Seal[];
  /** The requests whose questions are about it. */
  askedBy: number[];
}

export interface Collection {
  id: number;
  sequence: number;
  collectedFrom: string;
  collectedTo?: string;
  collectedBy: string;
  collectedById: number;
  nothingCollected: boolean;
  notes?: string;
  registeredBy?: string;
  createdAt: string;
  place?: string;
  /** The request that formalized it; absent while it is awaited. */
  request?: { id: number; documentReference: string; receivedOn: string };
  /** When the team was called out. */
  calledAt?: string;
  otherOfficers?: string;
  /** Users at the scene besides the collector. */
  team: { id: number; username: string; name: string }[];
  /** Audio the examiner recorded at the scene: their record, not evidence. */
  audioNotes: { fileId: number; filename: string; contentType: string; sizeBytes: number; sha256: string; createdAt: string }[];
}

export interface CaseOrigins {
  reportMode: ReportMode;
  collections: Collection[];
  vestiges: Vestige[];
}

export interface AwaitingCollection {
  collectionId: number;
  sequence: number;
  caseId: string;
  collectedFrom: string;
  collectedBy: string;
  nothingCollected: boolean;
  occurrence?: string;
}

export interface VestigeInput {
  description: string;
  sealNumber?: string;
}

export interface RegisterCollectionInput {
  occurrenceId?: number;
  occurrence?: CreateOccurrenceInput;
  description?: string;
  collectedFrom: string;
  collectedTo?: string;
  /** The collector; the signed-in user when omitted. */
  collectedById?: number;
  nothingCollected: boolean;
  notes?: string;
  reportMode: ReportMode;
  vestiges: VestigeInput[];
  calledAt?: string;
  otherOfficers?: string;
  team?: number[];
  /** The field note (scene note) the collection is registered from. */
  sceneNoteId?: number;
}

export interface RegisteredCollection {
  caseId: string;
  collectionId: number;
  occurrenceId: number;
  vestiges: Vestige[];
}

/** Where a file came from: one of the case's requests or collections, and the vestige it was taken from. */
export interface MaterialOrigin {
  requestId?: number;
  collectionId?: number;
  /** An analysis of the case that brought the material in (a reference document). */
  analysisId?: number;
  vestigeId?: number;
}

function authHeaders(): Record<string, string> {
  const token = getToken();
  return token ? { Authorization: `Bearer ${token}` } : {};
}

/** A call answered with no body (204). */
async function send(path: string, init: RequestInit): Promise<void> {
  const response = await apiFetch(`${apiBaseUrl}${path}`, {
    ...init,
    headers: { "Content-Type": "application/json", ...authHeaders() },
  });
  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(body.error ?? "The request could not be completed");
  }
}

const casePath = (caseId: string) => `/cases/${encodeURIComponent(caseId)}`;

/** Coordinators and fingerprint examiners: register a scene examined on call, opening its case. */
export async function registerCollection(input: RegisterCollectionInput): Promise<RegisteredCollection> {
  return requestApi<RegisteredCollection>("/collections", { method: "POST", headers: authHeaders(), body: JSON.stringify(input) });
}

export async function getCaseOrigins(caseId: string): Promise<CaseOrigins> {
  return requestApi<CaseOrigins>(`${casePath(caseId)}/collections`, { headers: authHeaders() });
}

/** Coordinator: the scenes still awaiting the request that formalizes them. */
export async function listAwaitingCollections(): Promise<AwaitingCollection[]> {
  return requestApi<AwaitingCollection[]>("/admin/collections/awaiting", { headers: authHeaders() });
}

/** Whoever works the case uses its own routes; a coordinator, who holds no analysis, the admin ones. */
const base = (caseId: string, asCoordinator: boolean) => (asCoordinator ? `/admin${casePath(caseId)}` : casePath(caseId));

export async function addVestige(
  caseId: string,
  input: VestigeInput & { requestId?: number; collectionId?: number },
  asCoordinator: boolean,
): Promise<Vestige> {
  return requestApi<Vestige>(`${base(caseId, asCoordinator)}/vestiges`, { method: "POST", headers: authHeaders(), body: JSON.stringify(input) });
}

export async function breakSeal(caseId: string, vestigeId: number, asCoordinator: boolean): Promise<void> {
  await send(`${base(caseId, asCoordinator)}/vestiges/${vestigeId}/seals/break`, { method: "POST" });
}

export async function reseal(caseId: string, vestigeId: number, number: string, asCoordinator: boolean): Promise<void> {
  await send(`${base(caseId, asCoordinator)}/vestiges/${vestigeId}/seals`, { method: "POST", body: JSON.stringify({ number }) });
}

/** Coordinator: whether the case's analyses are reported in one laudo or each in its own. */
export async function setReportMode(caseId: string, reportMode: ReportMode): Promise<void> {
  await send(`/admin${casePath(caseId)}/report-mode`, { method: "PUT", body: JSON.stringify({ reportMode }) });
}

/** One file a collection gathered, into its case, optionally from one of its vestiges. */
export async function receiveCollectionMaterial(
  caseId: string,
  collectionId: number,
  file: File,
  vestigeId: number | undefined,
  asCoordinator: boolean,
): Promise<number> {
  const form = new FormData();
  form.append("file", file);
  if (vestigeId) form.append("vestigeId", String(vestigeId));
  const response = await apiFetch(`${apiBaseUrl}${base(caseId, asCoordinator)}/collections/${collectionId}/material`, {
    method: "POST",
    headers: authHeaders(),
    body: form,
  });
  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(body.error ?? `Could not receive ${file.name}`);
  }
  return ((await response.json()) as { fileId: number }).fileId;
}

/** Whether an upload says where it came from, as the server requires. */
export function originChosen(o: MaterialOrigin | null): o is MaterialOrigin {
  return !!o && (!!o.requestId || !!o.collectionId || !!o.analysisId);
}

/** Adds an origin to an upload's form. */
export function appendOrigin(form: FormData, origin: MaterialOrigin): void {
  if (origin.requestId) form.append("requestId", String(origin.requestId));
  if (origin.collectionId) form.append("collectionId", String(origin.collectionId));
  if (origin.analysisId) form.append("analysisId", String(origin.analysisId));
  if (origin.vestigeId) form.append("vestigeId", String(origin.vestigeId));
}

/** An examiner working the case opens the comparison what it gave calls for, answering a request, and takes it. */
export async function openFollowUp(caseId: string, type: string, requestId: number): Promise<void> {
  await requestApi(`${casePath(caseId)}/analyses`, { method: "POST", headers: authHeaders(), body: JSON.stringify({ type, requestId }) });
}
