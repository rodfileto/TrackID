import { apiBaseUrl, requestApi } from "./api";
import { getToken } from "./auth";
import type { ThumbnailBox } from "./persons";

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

export interface CaseCodification {
  id: number;
  sequence: number;
  codificationType: string;
  caseFileId?: number;
  traceId: number;
  traceSequence: number;
  boxX1: number;
  boxY1: number;
  boxX2: number;
  boxY2: number;
  evidenceSequence: number;
  evidenceFileId: number;
  evidenceFilename: string;
}

/** One other biometric sample sharing a case cluster -- another case's
 * trace, or an enrolled person's KNOWN feature. decisionRole/automatic/
 * decidedBy describe the CONFIRMED decision chain directly linking one of
 * the cluster's local traces to this one, when there was a direct pair to
 * point to -- decisionRole is empty for a member reached only transitively
 * through other cluster members. automatic is true when decisionRole is
 * "SYSTEM" (an algorithmic match with no human confirmation yet), false for
 * a human-reviewed one (VERIFICATOR/REVIEWER/INCONSISTENCE).
 *
 * thumbnailFileId/thumbnailBox (QUESTIONED) say how to render this link as a
 * thumbnail via CaseTraceThumbnail, scoped to *this* link's own caseId, not
 * the case the cluster was requested for. identityFileId/contentType
 * (KNOWN) do the same via IdentityFileThumbnail, scoped to personId. */
export interface TraceLink {
  kind: "KNOWN" | "QUESTIONED";

  personId?: string;
  name?: string;
  identityFileId?: number;
  contentType?: string;

  caseId?: string;
  caseType?: string;
  description?: string;
  traceId?: number;
  thumbnailFileId?: number;
  thumbnailBox?: ThumbnailBox;

  decisionRole?: string;
  automatic: boolean;
  decidedBy?: string;
  confidence?: number;
}

/** One biometric cluster (see cases.CaseCluster) that at least one of a
 * case's codified traces resolved into -- traces of the same case sharing a
 * cluster are merged into localTraceIds instead of repeating the cluster
 * once per trace. */
export interface CaseCluster {
  clusterId: number;
  memberCount: number;
  localTraceIds: number[];
  links: TraceLink[] | null;
}

/** A face the detector found on an evidence image -- not saved until the
 * analyst sends it to createTraces. */
export interface FaceProposal {
  boxX1: number;
  boxY1: number;
  boxX2: number;
  boxY2: number;
  score: number;
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
  year?: string;
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
  if (params.year) query.set("year", params.year);

  const qs = query.toString();
  return requestApi<ListCasesResponse>(`/cases${qs ? `?${qs}` : ""}`, {
    headers: authHeaders(),
  });
}

export async function listCaseYears(): Promise<string[]> {
  const { years } = await requestApi<{ years: string[] }>("/cases/years", {
    headers: authHeaders(),
  });
  return years;
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

/** Where one side's embedding came from: already stored, or computed now
 * from the saved codification image or the evidence image. */
export type EmbeddingSource = "stored" | "codification_image" | "evidence";

export interface FaceComparison {
  /** Cosine similarity of the two face embeddings; 1 = identical. */
  similarity: number;
  embeddingType: string;
  modelVersion: string;
  faces: [
    { codificationId: number; source: EmbeddingSource },
    { codificationId: number; source: EmbeddingSource },
  ];
}

export class CompareFacesError extends Error {
  /** Set when no face could be found for this codification. */
  codificationId?: number;

  constructor(message: string, codificationId?: number) {
    super(message);
    this.codificationId = codificationId;
  }
}

export async function compareFaces(
  caseId: string,
  codificationIds: [number, number],
): Promise<FaceComparison> {
  const response = await fetch(
    `${apiBaseUrl}/cases/${encodeURIComponent(caseId)}/codifications/compare`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json", ...authHeaders() },
      body: JSON.stringify({ codificationIds }),
    },
  );

  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as {
      error?: string;
      codificationId?: number;
    };
    throw new CompareFacesError(
      body.error ?? "Could not compare faces",
      body.codificationId,
    );
  }

  return (await response.json()) as FaceComparison;
}

export async function listCaseCodifications(
  caseId: string,
): Promise<CaseCodification[]> {
  const { codifications } = await requestApi<{
    codifications: CaseCodification[];
  }>(`/cases/${encodeURIComponent(caseId)}/codifications`, {
    headers: authHeaders(),
  });
  return codifications;
}

export async function listCaseClusters(caseId: string): Promise<CaseCluster[]> {
  const { clusters } = await requestApi<{
    clusters: CaseCluster[];
  }>(`/cases/${encodeURIComponent(caseId)}/clusters`, {
    headers: authHeaders(),
  });
  return clusters;
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

export async function detectFaces(
  caseId: string,
  evidenceId: number,
): Promise<FaceProposal[]> {
  const { faces } = await requestApi<{ faces: FaceProposal[] }>(
    `/cases/${encodeURIComponent(caseId)}/evidences/${evidenceId}/detect-faces`,
    { method: "POST", headers: authHeaders() },
  );
  return faces;
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
