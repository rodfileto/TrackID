import { requestApi } from "./api";
import { getToken } from "./auth";
import type { PersonClusterMember } from "./persons";

/** One enrolled person a cluster has resolved to -- see cluster.SummaryPerson. */
export interface ClusterPerson {
  personId: string;
  personNumber: number;
  name: string;
}

export type ConflictKind = "ONE_BIOMETRIC_MANY_NAMES" | "ONE_IDENTITY_MANY_BIOMETRICS";
export type ConflictStrength = "REVIEWED" | "TWO_MODALITIES" | "ONE_MODALITY" | "INDIRECT" | "HUMAN_NEGATIVE";

export interface ConflictRecord {
  personNumber: number;
  name: string;
  documentType: string;
  documentNumber: string;
}

export interface ConflictRegister {
  personNumber: number;
  documentType: string;
  documentNumber: string;
  fiscalNumber?: string;
  registerNumber: string;
  name: string;
  birthDate?: string;
  parent1Name: string;
  parent2Name: string;
}

/** An identity conflict -- see cluster.Conflict. */
export interface IdentityConflict {
  kind: ConflictKind;
  strength: ConflictStrength;
  modalities: string[];
  /** What makes the records one identity, for ONE_IDENTITY_MANY_BIOMETRICS. */
  identityBasis?: "PERSON" | "FISCAL" | "NAME_BIRTH";
  clusterIds: number[];
  reviewedClusterIds?: number[];
  traces: number;
  persons: {
    personNumber: number;
    personId: string;
    names: string[];
    birthDates: string[];
    documents: { type: string; number: string }[];
    /** Their strongest reviewed link to case evidence; absent when people have confirmed none. */
    status?: { basis: "LEAD" | "LAUDO"; identified: boolean; conclusion?: string };
  }[];
  /** Every register of the people, to compare side by side. Filled for the pages only, never in the AI mode. */
  registers?: ConflictRegister[];
  comparisons: {
    modality: string;
    a: ConflictRecord;
    b: ConflictRecord;
    status?: string;
    settledBy?: string;
    score?: number;
    threshold?: number;
    /** The score was measured from the embeddings, not decided by the matcher. */
    measured?: boolean;
  }[];
}

/** One persisted biometric cluster's row in the general clusters listing --
 * see cluster.Summary. identified is true when at least one of its members
 * is a KNOWN (enrolled) feature, not just other case evidence; persons names
 * who it resolved to, when identified. */
export interface ClusterSummary {
  clusterId: number;
  modality: string;
  createdAt: string;
  memberCount: number;
  caseCount: number;
  identified: boolean;
  persons?: ClusterPerson[];
  conflicts?: { kind: ConflictKind; strength: ConflictStrength }[];
}

export interface ListClustersResponse {
  items: ClusterSummary[];
  total: number;
  page: number;
  pageSize: number;
  totalPages: number;
}

export interface ListClustersParams {
  page?: number;
  pageSize?: number;
  identifiedOnly?: boolean;
  conflictOnly?: boolean;
}

function authHeaders(): Record<string, string> {
  const token = getToken();
  return token ? { Authorization: `Bearer ${token}` } : {};
}

/** Lists every persisted biometric cluster, ordered by how many distinct
 * criminal cases it touches (descending). */
export async function listClusters(
  params: ListClustersParams = {},
): Promise<ListClustersResponse> {
  const query = new URLSearchParams();
  if (params.page) query.set("page", String(params.page));
  if (params.pageSize) query.set("page_size", String(params.pageSize));
  if (params.identifiedOnly) query.set("identified", "true");
  if (params.conflictOnly) query.set("conflict", "true");

  const qs = query.toString();
  return requestApi<ListClustersResponse>(`/clusters${qs ? `?${qs}` : ""}`, {
    headers: authHeaders(),
  });
}

export type GraphNodeKind = "person" | "cluster" | "trace";

/** One vertex of the identity/cluster/trace graph -- see cluster.GraphNode. */
export interface ClusterGraphNode {
  id: string;
  kind: GraphNodeKind;
  label: string;
  personId?: string;
  caseId?: string;
  modality?: string;
  size?: number;
}

export interface ClusterGraphEdge {
  source: string;
  target: string;
}

export interface ClusterGraph {
  nodes: ClusterGraphNode[];
  edges: ClusterGraphEdge[];
}

/** Top biometric clusters as a person -- cluster -- trace graph. */
/** The home page's illustration: the same graph, anonymous (no labels, names or ids), for every
 * signed-in user. */
export async function getOverviewGraph(): Promise<ClusterGraph> {
  return requestApi<ClusterGraph>("/overview/graph", { headers: authHeaders() });
}

export async function getClusterGraph(
  limit = 40,
  minMembers = 1,
): Promise<ClusterGraph> {
  return requestApi<ClusterGraph>(`/clusters/graph?limit=${limit}&min_members=${minMembers}`, {
    headers: authHeaders(),
  });
}

export type ClusterBasis = "AUTOMATIC" | "REVIEWED";
export type EdgeStatus = "PENDING_REVIEW" | "CONFIRMED" | "DISPUTED" | "REJECTED";

/** One member's place in its cluster's structure -- see cluster.DetailMember. */
export interface ClusterDetailMember {
  featureId: string;
  /** How many of the cluster's edges touch it. */
  degree: number;
  /** Removing it would split the cluster. */
  cutVertex: boolean;
  /** Its cluster in the other layer, when it has one. */
  otherClusterId?: number;
}

/** One biometric_decisions row of a pair's chain -- see cluster.DetailDecision. */
export interface ClusterDecision {
  role: "SYSTEM" | "VERIFICATOR" | "REVIEWER" | "INCONSISTENCE";
  decision: "POSITIVE" | "NEGATIVE" | "INCONCLUSIVE";
  decidedBy: string;
  confidence?: number;
  threshold?: number;
  analysisId?: number;
  analysisType?: string;
  conclusion?: string;
  decidedAt: string;
}

/** One decided pair touching the cluster -- see cluster.DetailPair. inside: both features are
 * members; edge: the pair links them in the cluster's layer; bridge: an edge whose removal
 * would split the cluster. */
export interface ClusterPair {
  featureA: string;
  featureB: string;
  inside: boolean;
  /** For a boundary pair: the outside sample's cluster in the same layer. */
  outsideClusterId?: number;
  status: EdgeStatus;
  settledBy?: string;
  reviewed: boolean;
  edge: boolean;
  bridge: boolean;
  score?: number;
  threshold?: number;
  chain: ClusterDecision[];
}

export interface ClusterMerge {
  fromClusterId: number;
  toClusterId: number;
  mergedAt: string;
  reason?: string;
}

/** One enrollment sample, of any modality, of a person the cluster resolves to, with its cluster
 * in the same layer -- see cluster.PersonSample. */
export interface ClusterPersonSample {
  personId: string;
  personNumber: number;
  name: string;
  registerNumber: string;
  birthDate?: string;
  parent1Name: string;
  parent2Name: string;
  identityFileId: number;
  fileType: string;
  modality: string;
  contentType?: string;
  featureId: string;
  clusterId?: number;
}

/** A cluster of another modality holding an enrollment sample of a person the cluster resolves
 * to: linked by enrollment, not by a comparison -- see cluster.LinkedCluster. */
export interface LinkedCluster {
  clusterId: number;
  modality: string;
  memberCount: number;
  questionedCount: number;
  personIds: string[];
}

/** One cluster in depth -- see api.ClusterDetailResponse. */
export interface ClusterDetail {
  clusterId: number;
  modality: string;
  basis: ClusterBasis;
  createdAt: string;
  updatedAt: string;
  members: ClusterDetailMember[];
  pairs: ClusterPair[];
  components: number;
  merges: ClusterMerge[];
  personSamples: ClusterPersonSample[];
  linkedClusters: LinkedCluster[];
  resolvedMembers: PersonClusterMember[];
  conflicts: IdentityConflict[];
}

/** One cluster, of either layer, in depth: its members, every decision touching it, what holds it
 * together, its merges and the persons it resolves to. */
export async function getCluster(clusterId: number): Promise<ClusterDetail> {
  return requestApi<ClusterDetail>(`/clusters/${clusterId}`, { headers: authHeaders() });
}
