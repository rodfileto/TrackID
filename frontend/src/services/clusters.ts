import { requestApi } from "./api";
import { getToken } from "./auth";

/** One enrolled person a cluster has resolved to -- see cluster.SummaryPerson. */
export interface ClusterPerson {
  personId: string;
  name: string;
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
export async function getClusterGraph(
  limit = 40,
  minMembers = 1,
): Promise<ClusterGraph> {
  return requestApi<ClusterGraph>(`/clusters/graph?limit=${limit}&min_members=${minMembers}`, {
    headers: authHeaders(),
  });
}
