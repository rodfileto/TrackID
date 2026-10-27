import { apiBaseUrl, requestApi } from "./api";
import { getToken } from "./auth";
import type { TraceLink } from "./cases";

export type AnalysisType = "FACIAL_REVIEW" | "FACIAL_COMPARISON";
export type AnalysisStage = "EXAMINATION" | "VERIFICATION" | "INCONSISTENCE" | "REPORT";
/** An examiner's part in an analysis, named after the decision role it records:
 * VERIFICATOR (first examination), REVIEWER (blind verification), INCONSISTENCE. */
export type AnalysisStep = "VERIFICATOR" | "REVIEWER" | "INCONSISTENCE";
export type DecisionValue = "POSITIVE" | "NEGATIVE" | "INCONCLUSIVE";
/** A comparison's (laudo's) graded conclusion on a pair, strongest support first. */
export type Conclusion =
  | "IDENTIFICATION"
  | "STRONG_SUPPORT"
  | "MODERATE_SUPPORT"
  | "LIMITED_SUPPORT"
  | "INCONCLUSIVE"
  | "LIMITED_EXCLUSION"
  | "EXCLUSION";

export const conclusions: Conclusion[] = [
  "IDENTIFICATION",
  "STRONG_SUPPORT",
  "MODERATE_SUPPORT",
  "LIMITED_SUPPORT",
  "INCONCLUSIVE",
  "LIMITED_EXCLUSION",
  "EXCLUSION",
];

/** The decision a conclusion maps onto, as the server's conclusion_decision. */
export function conclusionDecision(c: Conclusion): DecisionValue {
  if (c === "INCONCLUSIVE") return "INCONCLUSIVE";
  if (c === "LIMITED_EXCLUSION" || c === "EXCLUSION") return "NEGATIVE";
  return "POSITIVE";
}

/** How strong a reviewed link is: a lead, or a laudo at its level. */
export interface LinkStrength {
  basis: "LEAD" | "LAUDO";
  conclusion?: Conclusion;
  identified: boolean;
  /** A later comparison that left a lead standing: PENDING, DISPUTED or INCONCLUSIVE. */
  laterLaudo?: string;
}

export type Suitability = "SUITABLE" | "LEAD_ONLY" | "UNSUITABLE";
export const suitabilities: Suitability[] = ["SUITABLE", "LEAD_ONLY", "UNSUITABLE"];

export const analysisSteps: AnalysisStep[] = ["VERIFICATOR", "REVIEWER", "INCONSISTENCE"];

/** The step whose examiner works a stage (the first examiner issues the report). */
export function activeStep(stage: AnalysisStage): AnalysisStep {
  if (stage === "VERIFICATION") return "REVIEWER";
  if (stage === "INCONSISTENCE") return "INCONSISTENCE";
  return "VERIFICATOR";
}

export interface FeaturePair {
  featureA: string;
  featureB: string;
}

export interface AnalysisReport {
  id: number;
  /** The number of the report this one complements (a laudo complementar). */
  complements?: string;
  type: string;
  status: "DRAFT" | "ISSUED";
  number?: string;
  issuedBy?: string;
  issuedAt?: string;
}

export interface Analysis {
  id: number;
  sequence: number;
  type: AnalysisType;
  status: "OPEN" | "CLOSED";
  stage: AnalysisStage;
  requestedBy?: string;
  examiners: Partial<Record<AnalysisStep, { userId: number; username: string }>>;
  createdAt: string;
  closedAt?: string;
  pairs: FeaturePair[];
  /** The request it answers. */
  requestId?: number;
  report: AnalysisReport;
}

export interface Decision {
  step?: AnalysisStep;
  decision: DecisionValue;
  conclusion?: Conclusion;
  notes?: string;
  username?: string;
}

export interface PairView extends FeaturePair {
  questioned: TraceLink;
  counterpart: TraceLink;
  systemDecision?: string;
  score?: number;
  draft?: Decision;
  decisions: Decision[];
  disputed?: boolean;
}

export interface Worksheet {
  analysisId: number;
  caseId: string;
  type: AnalysisType;
  status: "OPEN" | "CLOSED";
  stage: AnalysisStage;
  yourStep?: AnalysisStep;
  /** The viewer works it blind (verification, inconsistence): the server sends only the images. */
  blind: boolean;
  pairs: PairView[];
  drafted: number;
}

export interface AssignedAnalysis {
  caseId: string;
  caseType: string;
  description: string;
  analysisId: number;
  sequence: number;
  type: AnalysisType;
  status: "OPEN" | "CLOSED";
  stage: AnalysisStage;
  step: AnalysisStep;
  yourTurn: boolean;
  /** Worked blind: no case type or description is sent. */
  blind: boolean;
  method: string;
  assignedAt: string;
}

export interface DistributionExaminer {
  userId: number;
  username: string;
  examinesFingerprint: boolean;
  examinesFacial: boolean;
  openWork: number;
}

export interface DistributionStatus {
  pending: { type: AnalysisType; stage: AnalysisStage; count: number }[];
  examiners: DistributionExaminer[];
}

export interface DistributionResult {
  assigned: {
    analysisId: number;
    caseId: string;
    sequence: number;
    type: AnalysisType;
    step: AnalysisStep;
    userId: number;
    username: string;
  }[];
  noExaminer: number[];
  stillPending: number;
  dryRun: boolean;
}

function authHeaders(): Record<string, string> {
  const token = getToken();
  return token ? { Authorization: `Bearer ${token}` } : {};
}

function analysisPath(caseId: string, analysisId: number): string {
  return `/cases/${encodeURIComponent(caseId)}/analyses/${analysisId}`;
}

export async function listCaseAnalyses(caseId: string): Promise<Analysis[]> {
  return requestApi<Analysis[]>(`/cases/${encodeURIComponent(caseId)}/analyses`, {
    headers: authHeaders(),
  });
}

/** Admin: open a FACIAL_COMPARISON on pairs (or another FACIAL_REVIEW, with none). */
export async function openAnalysis(
  caseId: string,
  type: AnalysisType,
  pairs: FeaturePair[],
  options: { requestId?: number; complements?: number } = {},
): Promise<Analysis[]> {
  return requestApi<Analysis[]>(`/admin/cases/${encodeURIComponent(caseId)}/analyses`, {
    method: "POST",
    headers: authHeaders(),
    body: JSON.stringify({ type, pairs, ...options }),
  });
}

/** One face of the analysis's pairs, by its feature id, as an object URL (the caller revokes it).
 * The server cuts a case's face to its box, so the workspace never needs the evidence itself. */
export async function getPairImageUrl(caseId: string, analysisId: number, featureId: string): Promise<string> {
  const response = await fetch(
    `${apiBaseUrl}${analysisPath(caseId, analysisId)}/pairs/image?feature=${encodeURIComponent(featureId)}`,
    { headers: authHeaders() },
  );
  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(body.error ?? "Could not load the image");
  }
  return URL.createObjectURL(await response.blob());
}

export async function getWorksheet(caseId: string, analysisId: number): Promise<Worksheet> {
  return requestApi<Worksheet>(`${analysisPath(caseId, analysisId)}/pairs`, {
    headers: authHeaders(),
  });
}

export async function saveDraft(
  caseId: string,
  analysisId: number,
  pair: FeaturePair,
  decision: DecisionValue,
  notes: string,
  conclusion?: Conclusion,
): Promise<void> {
  await requestApi<{ saved: boolean }>(`${analysisPath(caseId, analysisId)}/decisions`, {
    method: "PUT",
    headers: authHeaders(),
    body: JSON.stringify({ ...pair, decision, notes, conclusion }),
  });
}

/** The first examiner's value determination of a questioned face of the case. */
export async function assessTrace(
  caseId: string,
  analysisId: number,
  traceId: number,
  suitability: Suitability,
  notes: string,
): Promise<void> {
  await requestApi(`${analysisPath(caseId, analysisId)}/suitability`, {
    method: "PUT",
    headers: authHeaders(),
    body: JSON.stringify({ traceId, suitability, notes }),
  });
}

export async function submitStep(
  caseId: string,
  analysisId: number,
): Promise<{ decisions: number; stage: AnalysisStage }> {
  return requestApi(`${analysisPath(caseId, analysisId)}/submit`, {
    method: "POST",
    headers: authHeaders(),
  });
}

export async function issueReport(caseId: string, analysisId: number): Promise<AnalysisReport> {
  return requestApi<AnalysisReport>(`${analysisPath(caseId, analysisId)}/report`, {
    method: "POST",
    headers: authHeaders(),
  });
}

export interface ReportSummary {
  preliminary: boolean;
  reportType: string;
  title: string;
  number?: string;
  caseId: string;
  occurrence?: string;
  request?: string;
  questions: number;
  material: string;
  references: string;
  verification: string;
  previous: string[];
  examiners: string;
  results?: { questioned: string; counterpart: string; level: string; result: string; resultCode: string; lead: string }[];
  faces?: { face: string; suitability: string; result: string; resultCode: string; leads: string }[];
  notice?: string;
}

export interface ReportDelivery {
  deliveredOn: string;
  method: string;
  recipient: string;
  notes?: string;
  recordedBy?: string;
  recordedAt?: string;
}

export interface ReportVersion {
  version: number;
  generated: boolean;
  createdBy?: string;
  createdAt: string;
  stale: boolean;
}

export interface ReportOverview {
  status: "DRAFT" | "ISSUED";
  number?: string;
  summary: ReportSummary;
  draft?: ReportVersion;
  docxSha256?: string;
  pdfSha256?: string;
  deliveries: ReportDelivery[];
  /** Sections a laudo cannot be issued without. */
  required: string[];
}

export async function getReport(caseId: string, analysisId: number): Promise<ReportOverview> {
  return requestApi<ReportOverview>(`${analysisPath(caseId, analysisId)}/report`, { headers: authHeaders() });
}

async function failure(response: Response, fallback: string): Promise<Error> {
  const body = (await response.json().catch(() => ({}))) as { error?: string };
  return new Error(body.error ?? fallback);
}

/** The draft .docx to edit (generated on first use), with whether the data changed since. */
export async function getReportDraft(
  caseId: string,
  analysisId: number,
): Promise<{ bytes: ArrayBuffer; version: number; stale: boolean }> {
  const response = await fetch(`${apiBaseUrl}${analysisPath(caseId, analysisId)}/report/draft`, { headers: authHeaders() });
  if (!response.ok) throw await failure(response, "Could not load the draft");
  return {
    bytes: await response.arrayBuffer(),
    version: Number(response.headers.get("X-Report-Version") ?? 0),
    stale: response.headers.get("X-Report-Stale") === "true",
  };
}

/** Saves the examiner's edited .docx as the draft's next version. */
export async function saveReportDraft(caseId: string, analysisId: number, bytes: ArrayBuffer): Promise<ReportVersion> {
  const response = await fetch(`${apiBaseUrl}${analysisPath(caseId, analysisId)}/report/draft`, {
    method: "PUT",
    headers: { ...authHeaders(), "Content-Type": "application/vnd.openxmlformats-officedocument.wordprocessingml.document" },
    body: bytes,
  });
  if (!response.ok) throw await failure(response, "Could not save the draft");
  return (await response.json()) as ReportVersion;
}

export async function refreshReportDraft(caseId: string, analysisId: number): Promise<ReportVersion> {
  return requestApi<ReportVersion>(`${analysisPath(caseId, analysisId)}/report/draft/refresh`, {
    method: "POST",
    headers: authHeaders(),
  });
}

/** Downloads an issued report's PDF or .docx through the browser. */
export async function downloadReport(caseId: string, analysisId: number, format: "pdf" | "docx"): Promise<void> {
  const response = await fetch(`${apiBaseUrl}${analysisPath(caseId, analysisId)}/report/document?format=${format}`, {
    headers: authHeaders(),
  });
  if (!response.ok) throw await failure(response, "Could not download the report");
  const disposition = response.headers.get("Content-Disposition") ?? "";
  const name = decodeURIComponent(disposition.match(/filename\*=UTF-8''([^;]+)/)?.[1] ?? `report.${format}`);
  const url = URL.createObjectURL(await response.blob());
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  a.click();
  URL.revokeObjectURL(url);
}

export async function deliverReport(caseId: string, analysisId: number, delivery: ReportDelivery): Promise<ReportOverview> {
  return requestApi<ReportOverview>(`${analysisPath(caseId, analysisId)}/report/deliveries`, {
    method: "POST",
    headers: authHeaders(),
    body: JSON.stringify(delivery),
  });
}

export async function listMyAnalyses(): Promise<AssignedAnalysis[]> {
  return requestApi<AssignedAnalysis[]>("/me/analyses", { headers: authHeaders() });
}

export async function getDistributionStatus(): Promise<DistributionStatus> {
  return requestApi<DistributionStatus>("/admin/distribution", { headers: authHeaders() });
}

export async function distribute(
  count: number,
  type: AnalysisType | "",
  dryRun: boolean,
): Promise<DistributionResult> {
  return requestApi<DistributionResult>("/admin/distribution", {
    method: "POST",
    headers: authHeaders(),
    body: JSON.stringify({ count, type: type || undefined, dryRun }),
  });
}

/** Coordinator: the users who already decided (or drafted) the pairs of the analysis's current
 * step in another analysis or an import. Distribution avoids them; the assignment screen warns. */
export async function getPriorDeciders(analysisId: number): Promise<number[]> {
  const res = await requestApi<{ userIds: number[] }>(`/admin/analyses/${analysisId}/prior-deciders`, {
    headers: authHeaders(),
  });
  return res.userIds;
}

/** Coordinator: give the analysis's current step to userId, or unassign it (null). */
export async function assignAnalysis(analysisId: number, userId: number | null): Promise<void> {
  await requestApi(`/admin/analyses/${analysisId}/assignment`, {
    method: "PUT",
    headers: authHeaders(),
    body: JSON.stringify({ userId }),
  });
}

