import { apiBaseUrl, apiFetch, requestApi } from "./api";
import { getToken } from "./auth";

/** The analysis fields are absent once the reader no longer holds that analysis. */
export interface AppNotification {
  id: number;
  kind: "ANALYSIS_ASSIGNED";
  createdAt: string;
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
