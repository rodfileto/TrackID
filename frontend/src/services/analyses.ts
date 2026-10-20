import { apiBaseUrl, requestApi } from "./api";
import { getToken } from "./auth";
import type { TraceLink } from "./cases";

export type AnalysisType = "FACIAL_REVIEW" | "FACIAL_COMPARISON";
export type AnalysisStage = "EXAMINATION" | "VERIFICATION" | "INCONSISTENCE" | "REPORT";
/** An examiner's part in an analysis, named after the decision role it records:
 * VERIFICATOR (first examination), REVIEWER (blind verification), INCONSISTENCE. */
export type AnalysisStep = "VERIFICATOR" | "REVIEWER" | "INCONSISTENCE";
export type DecisionValue = "POSITIVE" | "NEGATIVE" | "INCONCLUSIVE";

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
  report: AnalysisReport;
}

export interface Decision {
  step?: AnalysisStep;
  decision: DecisionValue;
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
): Promise<Analysis[]> {
  return requestApi<Analysis[]>(`/admin/cases/${encodeURIComponent(caseId)}/analyses`, {
    method: "POST",
    headers: authHeaders(),
    body: JSON.stringify({ type, pairs }),
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
): Promise<void> {
  await requestApi<{ saved: boolean }>(`${analysisPath(caseId, analysisId)}/decisions`, {
    method: "PUT",
    headers: authHeaders(),
    body: JSON.stringify({ ...pair, decision, notes }),
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

