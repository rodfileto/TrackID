// Auth token management + API helpers for the TrackID backend
// (POST /api/v1/auth/login, /api/v1/auth/register).

const TOKEN_KEY = "trackid_token";

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY);
}

export function setToken(token: string): void {
  localStorage.setItem(TOKEN_KEY, token);
}

export function clearToken(): void {
  localStorage.removeItem(TOKEN_KEY);
}

export function isAuthenticated(): boolean {
  return getToken() !== null;
}

export function authHeaders(): Record<string, string> {
  const token = getToken();
  return token ? { Authorization: `Bearer ${token}` } : {};
}

async function parseDetail(response: Response): Promise<string> {
  const detail = await response.json().catch(() => null);
  return detail?.detail || `Request failed with status ${response.status}`;
}

export async function login(username: string, password: string): Promise<void> {
  const response = await fetch("/api/v1/auth/login", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ username, password }),
  });
  if (!response.ok) {
    throw new Error(await parseDetail(response));
  }
  const data: { token: string } = await response.json();
  setToken(data.token);
}

export async function register(username: string, password: string): Promise<void> {
  const response = await fetch("/api/v1/auth/register", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ username, password }),
  });
  if (!response.ok) {
    throw new Error(await parseDetail(response));
  }
}
