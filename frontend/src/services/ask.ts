import { apiBaseUrl, apiFetch } from "./api";
import { getToken } from "./auth";
import type { IdentityConflict } from "./clusters";

// The AI mode's API (api/ask_handler.go, docs/ask-plan.md). The conversation lives in the page: the
// server keeps a record of each question but no session.

export interface AskInfo {
  enabled: boolean;
  model?: string;
  host?: string;
  /** The model's server is outside the institute. */
  external: boolean;
}

export interface AskTurn {
  role: "user" | "assistant";
  text: string;
}

export interface BreakdownRow {
  key: string;
  count: number;
  sharePercent: number;
}

export interface ListedOccurrence {
  trackidNumber?: string;
  agency?: string;
  number?: string;
  inquiryNumber?: string;
  occurredOn?: string;
  attempted?: boolean;
  status: string;
  types: string[];
  placeName?: string;
  neighbourhood?: string;
  uf?: string;
  placeType?: string;
  description: string;
  cases: string[];
}

/** One tool the model called. result's shape depends on the tool (ask/tools.go). */
export interface AskStep {
  tool: string;
  arguments: unknown;
  result?: Record<string, unknown>;
  error?: string;
  durationMs: number;
}

/** Something the conversation found (ask.Entity). The page sends these back with the next question,
 * so "her cases" means the person the last answer found. */
export interface AskEntity {
  kind: "PERSON" | "CASE" | "OCCURRENCE" | "INQUIRY" | "CLUSTER";
  ref: string;
  label: string;
}

export type AskLink = "REVIEWED" | "LEAD";

export interface AskFoundPerson {
  personNumber: number;
  names: string[];
  birthDates: string[];
  documents: { type: string; number: string }[];
  cases: number;
  reviewedCases: number;
}

export interface AskPersonSearch {
  rowCount: number;
  rows: AskFoundPerson[];
  truncated: boolean;
}

export interface AskLinkedPerson {
  personNumber: number;
  names: string[];
  birthDate?: string;
  link: AskLink;
  /** Others matched to the very same trace: one face or print under several identities, or a false match. */
  sameTraceAs?: number[];
  clusterIds?: number[];
}

export interface AskIdentityConflicts {
  total: number;
  byKind: Record<string, number>;
  byStrength: Record<string, number>;
  rowCount: number;
  rows: IdentityConflict[];
  truncated: boolean;
}

export interface AskCluster {
  clusterId: number;
  basis: "AUTOMATIC" | "REVIEWED";
  modality: string;
  members: number;
  merges: number;
  persons: { personNumber: number; names: string[]; birthDate?: string; samples: number }[];
  cases: { caseId: string; caseType: string; modality: string; traces: number; occurrence?: AskOccurrence }[];
  linkedClusters: { clusterId: number; modality: string; memberCount: number; questionedCount: number }[];
  conflicts: IdentityConflict[];
}

export interface AskPersonClusters {
  person: AskFoundPerson;
  rowCount: number;
  rows: { clusterId: number; basis: "AUTOMATIC" | "REVIEWED"; modality: string; members: number; traces: number }[];
  conflicts: IdentityConflict[];
}

export type AskIdentityEvidence = "REVIEWED" | "TWO_MODALITIES" | "ONE_MODALITY" | "INDIRECT" | "HUMAN_NEGATIVE";

export interface AskPersonIdentities {
  person: AskFoundPerson;
  ownNames: number;
  rowCount: number;
  weakCandidates: number;
  truncated: boolean;
  rows: {
    person: AskFoundPerson;
    evidence: AskIdentityEvidence;
    sharedTraces: number;
    sharesName: boolean;
    sharesBirthDate: boolean;
    sharesParent: boolean;
    comparisons: {
      modality: string;
      sameCluster: boolean;
      reviewed: boolean;
      decision?: string;
      score?: number;
      threshold?: number;
      examinerDecisions?: string[];
      matches?: string[];
    }[];
  }[];
}

export interface AskOccurrence {
  id: number;
  trackidNumber?: string;
  agency?: string;
  number?: string;
  inquiryNumber?: string;
  occurredFrom?: string;
  occurredTo?: string;
  reportedAt?: string;
  registeredAt: string;
  attempted?: boolean;
  offenderCount?: number;
  victimCount?: number;
  status: string;
  types: string[];
  description: string;
  placeName?: string;
  address?: string;
  neighbourhood?: string;
  municipality?: string;
  uf?: string;
  placeType?: string;
  precision?: string;
  lat?: number;
  lon?: number;
  cases?: { caseId: string; caseType: string; modality: string; persons: AskLinkedPerson[] }[];
}

export interface AskPersonCases {
  person: AskFoundPerson;
  rowCount: number;
  reviewedCount: number;
  leadCount: number;
  rows: { caseId: string; caseType: string; modality: string; link: AskLink; occurrence?: AskOccurrence; clusterIds?: number[] }[];
  truncated: boolean;
}

export interface AskCaseDetail {
  caseId: string;
  caseType: string;
  modality: string;
  description: string;
  occurrence?: AskOccurrence;
  requests: { sequence: number; authority: string; reference: string; documentDate?: string; receivedOn: string; dueOn?: string }[];
  reports: { type: string; number?: string; issuedOn?: string }[];
  persons: AskLinkedPerson[];
}

export interface AskInquiry {
  inquiryNumber: string;
  rowCount: number;
  rows: AskOccurrence[];
  truncated: boolean;
}

export interface AskOccurrenceSearch {
  rowCount: number;
  rows: {
    id: number;
    trackidNumber?: string;
    agency?: string;
    number?: string;
    inquiryNumber?: string;
    occurredFrom?: string;
    status: string;
    placeName?: string;
    neighbourhood?: string;
    types: string[];
  }[];
}

export interface AskNear {
  centerLat: number;
  centerLon: number;
  radiusM: number;
  rowCount: number;
  truncated: boolean;
  municipalityOnlyLeftOut: number;
  undatedLeftOut: number;
  rows: {
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
    lat: number;
    lon: number;
    distanceM: number;
    linkedCases: number;
  }[];
}

export interface AskAnswer {
  id: number;
  answer: string;
  steps: AskStep[];
  entities: AskEntity[];
  /** Numbers in the answer that no tool returned. */
  unverifiedNumbers: string[];
  model: string;
  external: boolean;
  usage: { inputTokens: number; outputTokens: number };
  durationMs: number;
}

/** A refused or failed question. code is "real_data", "refused" or "truncated" when known. */
export class AskError extends Error {
  code?: string;
  constructor(message: string, code?: string) {
    super(message);
    this.code = code;
  }
}

function authHeaders(): Record<string, string> {
  const token = getToken();
  return token ? { Authorization: `Bearer ${token}` } : {};
}

export async function getAskInfo(): Promise<AskInfo> {
  const response = await apiFetch(`${apiBaseUrl}/ask`, { headers: authHeaders() });
  if (!response.ok) throw new AskError("The request could not be completed");
  return (await response.json()) as AskInfo;
}

export async function askQuestion(question: string, history: AskTurn[], focus: AskEntity[], signal?: AbortSignal): Promise<AskAnswer> {
  const response = await apiFetch(`${apiBaseUrl}/ask`, {
    method: "POST",
    headers: { "Content-Type": "application/json", ...authHeaders() },
    body: JSON.stringify({ question, history, focus }),
    signal,
  });
  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as { error?: string; code?: string };
    throw new AskError(body.error ?? "The request could not be completed", body.code);
  }
  return (await response.json()) as AskAnswer;
}
