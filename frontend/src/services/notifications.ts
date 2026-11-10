import { apiBaseUrl, apiFetch, requestApi } from "./api";
import { getToken } from "./auth";

/** How many of one change a followed person or cluster went through. Refs names the first cases (CASE_LINKED),
 * clusters (CLUSTER_JOINED) or merge target (MERGED). A deployment adds its own changes. */
export interface ChangeCount {
  change: string;
  count: number;
  refs?: string[];
}

/** What a followed person or cluster went through, while the reader is an intelligence analyst. */
export interface SubjectChanges {
  subjectKind: "PERSON" | "CLUSTER";
  subjectId: string;
  personNumber?: number;
  name?: string;
  /** The cluster no longer exists: it was merged into this one. */
  mergedInto?: number;
  changes: ChangeCount[];
}

/** The analysis fields are absent once the reader no longer holds that analysis. */
export interface AppNotification {
  id: number;
  kind: "ANALYSIS_ASSIGNED" | "PERSON_CHANGED" | "CLUSTER_CHANGED";
  createdAt: string;
  /** A grouped notification moves to the top when it is extended. */
  updatedAt: string;
  changes?: SubjectChanges;
  read: boolean;
  caseId?: string;
  analysisId?: number;
  analysisType?: string;
  step?: string;
  /** Worked blind right now: show no case identity. */
  blind?: boolean;
}

export interface NotificationPage {
  items: AppNotification[];
  unread: number;
}

function authHeaders(): Record<string, string> {
  const token = getToken();
  return token ? { Authorization: `Bearer ${token}` } : {};
}

export async function listNotifications(): Promise<NotificationPage> {
  return requestApi<NotificationPage>("/notifications", { headers: authHeaders() });
}

async function put(path: string): Promise<void> {
  const response = await apiFetch(`${apiBaseUrl}${path}`, { method: "PUT", headers: authHeaders() });
  if (!response.ok) throw new Error("The notification could not be updated");
}

export function markNotificationRead(id: number): Promise<void> {
  return put(`/notifications/${id}/read`);
}

export function markAllNotificationsRead(): Promise<void> {
  return put("/notifications/read-all");
}
