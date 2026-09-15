import { apiBaseUrl, requestApi } from "./api";
import { getToken } from "./auth";

export interface Case {
  caseId: string;
  caseType: string;
  description: string;
}

export interface Evidence {
  id: number;
  category: string;
  mediaType?: string;
  filename: string;
  storageRef?: string;
  contentType?: string;
  sizeBytes: number;
  createdAt: string;
}

export interface CaseDetail {
  caseId: string;
  caseType: string;
  description: string;
  evidences: Evidence[];
}

export interface Trace {
  id: number;
  sequence: number;
  traceType: string;
  boxX1: number;
  boxY1: number;
  boxX2: number;
  boxY2: number;
  score: number;
  featureId: number;
}

export interface TraceInput {
  boxX1: number;
  boxY1: number;
  boxX2: number;
  boxY2: number;
  score?: number;
}

export interface Point {
  id: number;
  sequence: number;
  x: number;
  y: number;
  pointType?: string;
  angle?: number;
}

export interface PointInput {
  x: number;
  y: number;
  pointType?: string;
  angle?: number;
}

export interface ListCasesResponse {
  items: Case[];
  total: number;
  page: number;
  pageSize: number;
  totalPages: number;
}

export interface ListCasesParams {
  page?: number;
  pageSize?: number;
  caseType?: string;
  q?: string;
}

export interface CreateCaseInput {
  caseType: string;
  description: string;
}

function authHeaders(): Record<string, string> {
  const token = getToken();
  return token ? { Authorization: `Bearer ${token}` } : {};
}

export async function listCases(
  params: ListCasesParams = {},
): Promise<ListCasesResponse> {
  const query = new URLSearchParams();
  if (params.page) query.set("page", String(params.page));
  if (params.pageSize) query.set("page_size", String(params.pageSize));
  if (params.caseType) query.set("case_type", params.caseType);
  if (params.q) query.set("q", params.q);

  const qs = query.toString();
  return requestApi<ListCasesResponse>(`/cases${qs ? `?${qs}` : ""}`, {
    headers: authHeaders(),
  });
}

export async function createCase(input: CreateCaseInput): Promise<Case> {
  return requestApi<Case>("/cases", {
    method: "POST",
    body: JSON.stringify(input),
    headers: authHeaders(),
  });
}

export async function getCase(caseId: string): Promise<CaseDetail> {
  return requestApi<CaseDetail>(`/cases/${encodeURIComponent(caseId)}`, {
    headers: authHeaders(),
  });
}

export async function addEvidence(
  caseId: string,
  file: File,
): Promise<Evidence> {
  const form = new FormData();
  form.append("file", file);

  const token = getToken();
  const response = await fetch(
    `${apiBaseUrl}/cases/${encodeURIComponent(caseId)}/evidences`,
    {
      method: "POST",
      headers: token ? { Authorization: `Bearer ${token}` } : {},
      body: form,
    },
  );

  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as {
      error?: string;
    };
    throw new Error(body.error ?? "Could not upload evidence");
  }

  return (await response.json()) as Evidence;
}

export async function deleteEvidence(
  caseId: string,
  evidenceId: number,
): Promise<void> {
  await requestApi<Record<string, never>>(
    `/cases/${encodeURIComponent(caseId)}/evidences/${evidenceId}`,
    { method: "DELETE", headers: authHeaders() },
  );
}

export async function getEvidenceObjectUrl(
  caseId: string,
  evidenceId: number,
): Promise<string> {
  const token = getToken();
  const response = await fetch(
    `${apiBaseUrl}/cases/${encodeURIComponent(caseId)}/evidences/${evidenceId}/download`,
    {
      headers: token ? { Authorization: `Bearer ${token}` } : {},
    },
  );

  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as {
      error?: string;
    };
    throw new Error(body.error ?? "Could not load evidence");
  }

  const blob = await response.blob();
  return URL.createObjectURL(blob);
}

export async function listTraces(
  caseId: string,
  evidenceId: number,
): Promise<Trace[]> {
  const { traces } = await requestApi<{ traces: Trace[] }>(
    `/cases/${encodeURIComponent(caseId)}/evidences/${evidenceId}/traces`,
    { headers: authHeaders() },
  );
  return traces;
}

export async function createTraces(
  caseId: string,
  evidenceId: number,
  traces: TraceInput[],
): Promise<Trace[]> {
  const { traces: created } = await requestApi<{ traces: Trace[] }>(
    `/cases/${encodeURIComponent(caseId)}/evidences/${evidenceId}/traces`,
    {
      method: "POST",
      body: JSON.stringify({ traces }),
      headers: authHeaders(),
    },
  );
  return created;
}

export async function deleteTrace(
  caseId: string,
  evidenceId: number,
  traceId: number,
): Promise<void> {
  await requestApi<Record<string, never>>(
    `/cases/${encodeURIComponent(caseId)}/evidences/${evidenceId}/traces/${traceId}`,
    { method: "DELETE", headers: authHeaders() },
  );
}

function pointsPath(caseId: string, evidenceId: number, traceId: number): string {
  return `/cases/${encodeURIComponent(caseId)}/evidences/${evidenceId}/traces/${traceId}/points`;
}

export async function listPoints(
  caseId: string,
  evidenceId: number,
  traceId: number,
): Promise<Point[]> {
  const { points } = await requestApi<{ points: Point[] }>(
    pointsPath(caseId, evidenceId, traceId),
    { headers: authHeaders() },
  );
  return points;
}

export async function addPoints(
  caseId: string,
  evidenceId: number,
  traceId: number,
  points: PointInput[],
): Promise<Point[]> {
  const { points: created } = await requestApi<{ points: Point[] }>(
    pointsPath(caseId, evidenceId, traceId),
    {
      method: "POST",
      body: JSON.stringify({ points }),
      headers: authHeaders(),
    },
  );
  return created;
}

export async function updatePoint(
  caseId: string,
  evidenceId: number,
  traceId: number,
  pointId: number,
  point: PointInput,
): Promise<void> {
  await requestApi<Record<string, never>>(
    `${pointsPath(caseId, evidenceId, traceId)}/${pointId}`,
    {
      method: "PUT",
      body: JSON.stringify(point),
      headers: authHeaders(),
    },
  );
}

export async function deletePoint(
  caseId: string,
  evidenceId: number,
  traceId: number,
  pointId: number,
): Promise<void> {
  await requestApi<Record<string, never>>(
    `${pointsPath(caseId, evidenceId, traceId)}/${pointId}`,
    { method: "DELETE", headers: authHeaders() },
  );
}

export interface CodificationImage {
  id: number;
  category: string;
  mediaType?: string;
  filename: string;
  storageRef?: string;
  contentType?: string;
  sizeBytes: number;
  createdAt: string;
}

export async function saveCodificationImage(
  caseId: string,
  evidenceId: number,
  traceId: number,
  blob: Blob,
): Promise<CodificationImage> {
  const form = new FormData();
  form.append("file", blob, "codification.png");

  const token = getToken();
  const response = await fetch(
    `${apiBaseUrl}/cases/${encodeURIComponent(caseId)}/evidences/${evidenceId}/traces/${traceId}/codification-image`,
    {
      method: "POST",
      headers: token ? { Authorization: `Bearer ${token}` } : {},
      body: form,
    },
  );

  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as {
      error?: string;
    };
    throw new Error(body.error ?? "Could not save codification image");
  }

  return (await response.json()) as CodificationImage;
}
