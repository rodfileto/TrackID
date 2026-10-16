import { requestApi } from "./api";
import { getToken, type UserProfile } from "./auth";
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

/** Admin: give the analysis's current step to userId, or unassign it (null). */
export async function assignAnalysis(analysisId: number, userId: number | null): Promise<void> {
  await requestApi(`/admin/analyses/${analysisId}/assignment`, {
    method: "PUT",
    headers: authHeaders(),
    body: JSON.stringify({ userId }),
  });
}

/** Admin: every account, to pick an examiner from. */
export async function listUsers(): Promise<UserProfile[]> {
  return requestApi<UserProfile[]>("/admin/users", { headers: authHeaders() });
}
