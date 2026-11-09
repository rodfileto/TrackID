import { requestApi } from "./api";
import { getToken } from "./auth";

/** An intelligence operation -- see operations.Operation. */
export interface Operation {
  id: number;
  slug: string;
  name: string;
  description: string;
  status: "ACTIVE" | "CLOSED";
  createdAt: string;
  updatedAt: string;
  closedAt?: string;
}

function authHeaders(): Record<string, string> {
  const token = getToken();
  return token ? { Authorization: `Bearer ${token}` } : {};
}

export function listOperations(): Promise<Operation[]> {
  return requestApi<Operation[]>("/operations", { headers: authHeaders() });
}

export function getOperation(slug: string): Promise<Operation> {
  return requestApi<Operation>(`/operations/${encodeURIComponent(slug)}`, { headers: authHeaders() });
}

export function createOperation(input: { name: string; description: string }): Promise<Operation> {
  return requestApi<Operation>("/operations", {
    method: "POST",
    headers: authHeaders(),
    body: JSON.stringify(input),
  });
}

export function updateOperation(
  slug: string,
  input: { name: string; description: string; status: Operation["status"] },
): Promise<Operation> {
  return requestApi<Operation>(`/operations/${encodeURIComponent(slug)}`, {
    method: "PUT",
    headers: authHeaders(),
    body: JSON.stringify(input),
  });
}
