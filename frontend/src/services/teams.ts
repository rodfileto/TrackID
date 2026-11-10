import { apiBaseUrl, apiFetch, requestApi, type ErrorResponse } from "./api";
import { getToken } from "./auth";

/** The signed-in user's Teams webhook. The URL is never sent back once set. */
export interface TeamsWebhookStatus {
  /** Teams is configured on this server. */
  available: boolean;
  configured: boolean;
  setAt?: string;
  lastError?: string;
  failedAt?: string;
}

function headers(): Record<string, string> {
  const token = getToken();
  return { "Content-Type": "application/json", ...(token ? { Authorization: `Bearer ${token}` } : {}) };
}

export function getTeamsWebhook(): Promise<TeamsWebhookStatus> {
  return requestApi<TeamsWebhookStatus>("/me/teams-webhook", { headers: headers() });
}

export function setTeamsWebhook(url: string): Promise<TeamsWebhookStatus> {
  return requestApi<TeamsWebhookStatus>("/me/teams-webhook", {
    method: "PUT",
    headers: headers(),
    body: JSON.stringify({ url }),
  });
}

async function noContent(path: string, method: string): Promise<void> {
  const response = await apiFetch(`${apiBaseUrl}${path}`, { method, headers: headers() });
  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as ErrorResponse;
    throw new Error(body.error ?? "The request could not be completed");
  }
}

export function clearTeamsWebhook(): Promise<void> {
  return noContent("/me/teams-webhook", "DELETE");
}

export function testTeamsWebhook(): Promise<void> {
  return noContent("/me/teams-webhook/test", "POST");
}
