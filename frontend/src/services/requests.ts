import { apiBaseUrl, apiFetch, requestApi } from "./api";
import { getToken } from "./auth";
import type { Suitability } from "./analyses";
import type { CreateOccurrenceInput } from "./occurrences";

/** Another number a request's document goes by, labelled with the system it belongs to. */
export interface RequestReference {
  label: string;
  number: string;
}

/** A request: an authority asking the institute to examine material, with its questions. */
export interface CaseRequest {
  id: number;
  sequence: number;
  authority: string;
  documentReference: string;
  documentDate?: string;
  receivedOn: string;
  dueOn?: string;
  overdue: boolean;
  citedReference?: string;
  citedInquiryNumber?: string;
  occurrence?: { id: number; number?: string; agency?: string; occurrenceNumber?: string; inquiryNumber?: string };
  notes?: string;
  registeredBy?: string;
  createdAt: string;
  questions: string[];
  files: { id: number; filename: string; sha256: string; contentType?: string; sizeBytes: number; addedAt: string }[];
  /** Every case the request opened or joined, this one included. */
  cases: { caseId: string; modality: string }[];
  /** The analyses it asked of this case, by type. */
  tasks: string[];
  references: RequestReference[];
}

/** A request in the list of every request. */
export interface RequestSummary {
  id: number;
  authority: string;
  documentReference: string;
  receivedOn: string;
  dueOn?: string;
  openAnalyses: number;
  overdue: boolean;
  citedReference?: string;
  citedInquiryNumber?: string;
  occurrenceNumber?: string;
  caseIds: string[];
  /** The analyses it asked, by type, across its cases. */
  tasks: string[];
  references: RequestReference[];
}

/** One case a request opens (description) or joins (caseId), and the task it asks of it. */
export interface RequestCaseInput {
  caseId?: string;
  task: string;
  description?: string;
}

/** A request as it arrives, with the occurrence it cites and the cases it opens. */
export interface RegisterRequestInput extends Omit<CaseRequestInput, "fileIds"> {
  occurrenceId?: number;
  occurrence?: CreateOccurrenceInput;
  caseType?: string;
  cases: RequestCaseInput[];
}

export interface RegisteredRequest {
  requestId: number;
  occurrence?: CaseRequest["occurrence"];
  cases: { caseId: string; caseType: string; modality: string; description: string }[];
}

export interface CaseRequestInput {
  authority: string;
  documentReference: string;
  documentDate?: string;
  receivedOn: string;
  dueOn?: string;
  citedReference?: string;
  citedInquiryNumber?: string;
  notes?: string;
  questions: string[];
  references?: RequestReference[];
  fileIds: number[];
  /** The task the request asks of the case (createCaseRequest only). */
  task?: string;
}

/** One face of a case with its role and the examiner's value determination. */
export interface MaterialTrace {
  traceId: number;
  featureId: string;
  label: string;
  evidenceFileId?: number;
  cropFileId?: number;
  box?: { x1: number; y1: number; x2: number; y2: number };
  role: "QUESTIONED" | "REFERENCE";
  suitability?: Suitability;
  suitabilityNotes?: string;
  suitabilityBy?: string;
  suitabilityAt?: string;
}

/** The occurrence and inquiry numbers a request cites, for display. */
export function citedNumbers(r: { citedReference?: string; citedInquiryNumber?: string }): string {
  return [r.citedReference, r.citedInquiryNumber && `IPL ${r.citedInquiryNumber}`].filter(Boolean).join(" · ");
}

function authHeaders(): Record<string, string> {
  const token = getToken();
  return token ? { Authorization: `Bearer ${token}` } : {};
}

export async function listCaseRequests(caseId: string): Promise<CaseRequest[]> {
  return requestApi<CaseRequest[]>(`/cases/${encodeURIComponent(caseId)}/requests`, { headers: authHeaders() });
}

/** Coordinator: every request, soonest due first. */
/** Reads "Label: number" lines; a line with no colon has no label, which the server refuses. */
export function parseReferences(text: string): RequestReference[] {
  return text
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean)
    .map((line) => {
      const colon = line.indexOf(":");
      return colon < 0 ? { label: "", number: line } : { label: line.slice(0, colon).trim(), number: line.slice(colon + 1).trim() };
    });
}

export function formatReferences(references: RequestReference[]): string {
  return references.map((r) => `${r.label}: ${r.number}`).join(" · ");
}

export async function listRequests(reference = ""): Promise<RequestSummary[]> {
  const query = reference ? `?reference=${encodeURIComponent(reference)}` : "";
  return requestApi<RequestSummary[]>(`/admin/requests${query}`, { headers: authHeaders() });
}

/** Coordinator: register a request as it arrives, opening its cases and its occurrence. */
export async function registerRequest(input: RegisterRequestInput): Promise<RegisteredRequest> {
  return requestApi<RegisteredRequest>("/admin/requests", {
    method: "POST",
    headers: authHeaders(),
    body: JSON.stringify(input),
  });
}

/** Coordinator: receive one file a request sent in, into one of its cases. */
export async function receiveMaterial(caseId: string, requestId: number, file: File): Promise<number> {
  const form = new FormData();
  form.append("file", file);
  const response = await apiFetch(`${apiBaseUrl}/admin/cases/${encodeURIComponent(caseId)}/requests/${requestId}/material`, {
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

/** Coordinator: register a later request on a case already open. */
export async function createCaseRequest(caseId: string, input: CaseRequestInput): Promise<CaseRequest> {
  return requestApi<CaseRequest>(`/admin/cases/${encodeURIComponent(caseId)}/requests`, {
    method: "POST",
    headers: authHeaders(),
    body: JSON.stringify(input),
  });
}

/** Coordinator: mark more evidence files as sent in by a request. */
export async function attachRequestFiles(caseId: string, requestId: number, fileIds: number[]): Promise<CaseRequest[]> {
  return requestApi<CaseRequest[]>(`/admin/cases/${encodeURIComponent(caseId)}/requests/${requestId}/files`, {
    method: "POST",
    headers: authHeaders(),
    body: JSON.stringify({ fileIds }),
  });
}

export async function listCaseMaterial(caseId: string): Promise<MaterialTrace[]> {
  return requestApi<MaterialTrace[]>(`/cases/${encodeURIComponent(caseId)}/material`, { headers: authHeaders() });
}

/** Mark a face as questioned material or as a reference sent in for comparison. */
export async function setTraceRole(caseId: string, traceId: number, role: MaterialTrace["role"]): Promise<void> {
  await requestApi(`/cases/${encodeURIComponent(caseId)}/traces/${traceId}/role`, {
    method: "PUT",
    headers: authHeaders(),
    body: JSON.stringify({ role }),
  });
}
