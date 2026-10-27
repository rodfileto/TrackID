import { requestApi } from "./api";
import { getToken } from "./auth";
import type { Suitability } from "./analyses";

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
  occurrence?: { id: number; number?: string; agency?: string; occurrenceNumber?: string };
  notes?: string;
  registeredBy?: string;
  createdAt: string;
  questions: string[];
  files: { id: number; filename: string; sha256: string; contentType?: string; sizeBytes: number; addedAt: string }[];
}

export interface CaseRequestInput {
  authority: string;
  documentReference: string;
  documentDate?: string;
  receivedOn: string;
  dueOn?: string;
  citedReference?: string;
  notes?: string;
  questions: string[];
  fileIds: number[];
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

function authHeaders(): Record<string, string> {
  const token = getToken();
  return token ? { Authorization: `Bearer ${token}` } : {};
}

export async function listCaseRequests(caseId: string): Promise<CaseRequest[]> {
  return requestApi<CaseRequest[]>(`/cases/${encodeURIComponent(caseId)}/requests`, { headers: authHeaders() });
}

/** Coordinator: register a request on a case. */
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
