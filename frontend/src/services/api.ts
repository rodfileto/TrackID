export const apiBaseUrl = import.meta.env.VITE_API_URL ?? "/api";

export interface ErrorResponse {
  error?: string;
  code?: string;
}

interface SessionHandlers {
  /** Gets a new token in place of one the server refused. */
  renew: (refusedToken: string) => Promise<string>;
  /** The server says the signed-in account was deactivated. */
  deactivated: () => void;
}

let session: SessionHandlers | null = null;

/** Sets who deals with a refused session (services/auth.ts). */
export function setSessionHandlers(handlers: SessionHandlers): void {
  session = handlers;
}

/** fetch for the API; every call to it goes through here. A request refused for its token waits
 * until the user signs in again and is then sent once more, so the page that made it neither sees
 * an error nor loses what it holds. The server refuses before running the handler, so nothing has
 * happened yet. A request with no token is left alone: that is a sign-in with a wrong password.
 * A deactivated account (api.ActiveUser) cannot renew anything, so its session just ends. */
export async function apiFetch(input: string, init?: RequestInit): Promise<Response> {
  const response = await fetch(input, init);
  if ((response.status !== 401 && response.status !== 403) || !session) return response;

  const headers = new Headers(init?.headers);
  const refused = headers.get("Authorization")?.replace(/^Bearer /, "");
  if (!refused) return response;

  if (response.status === 403) {
    const body = (await response.clone().json().catch(() => ({}))) as ErrorResponse;
    if (body.code !== "account_deactivated") return response;
    session.deactivated();
    throw new Error(body.error);
  }

  headers.set("Authorization", `Bearer ${await session.renew(refused)}`);
  return fetch(input, { ...init, headers });
}

export async function requestApi<T>(
  path: string,
  options?: RequestInit,
): Promise<T> {
  const response = await apiFetch(`${apiBaseUrl}${path}`, {
    headers: {
      "Content-Type": "application/json",
      ...options?.headers,
    },
    ...options,
  });

  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as ErrorResponse;
    throw new Error(body.error ?? "The request could not be completed");
  }

  if (response.status === 204) return undefined as T;
  return (await response.json()) as T;
}
