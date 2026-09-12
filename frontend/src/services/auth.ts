export interface UserProfile {
  id: number;
  nome: string;
  ultimo_nome: string;
  matricula: string;
  cargo: string;
  username: string;
  email: string;
}

interface LoginResponse {
  token: string;
  user: UserProfile;
}

interface RegisterRequest {
  nome: string;
  ultimo_nome: string;
  matricula: string;
  cargo: string;
  username: string;
  password: string;
}

interface ErrorResponse {
  error?: string;
}

const apiBaseUrl = import.meta.env.VITE_API_URL ?? "/api";
const sessionTokenKey = "trackid.session.token";
const localTokenKey = "trackid.local.token";
const sessionUserKey = "trackid.session.user";
const localUserKey = "trackid.local.user";

async function requestApi<T>(path: string, options?: RequestInit): Promise<T> {
  const response = await fetch(`${apiBaseUrl}${path}`, {
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

  return (await response.json()) as T;
}

export async function login(
  matricula: string,
  password: string,
  keepLoggedIn: boolean,
): Promise<UserProfile> {
  const response = await requestApi<LoginResponse>("/auth/login", {
    method: "POST",
    body: JSON.stringify({ matricula, password }),
  });

  const tokenStorage = keepLoggedIn ? localStorage : sessionStorage;
  const userStorage = keepLoggedIn ? localStorage : sessionStorage;
  const tokenKey = keepLoggedIn ? localTokenKey : sessionTokenKey;
  const userKey = keepLoggedIn ? localUserKey : sessionUserKey;

  tokenStorage.setItem(tokenKey, response.token);
  userStorage.setItem(userKey, JSON.stringify(response.user));

  return response.user;
}

export async function register(request: RegisterRequest): Promise<UserProfile> {
  return requestApi<UserProfile>("/auth/register", {
    method: "POST",
    body: JSON.stringify(request),
  });
}

export function getToken(): string | null {
  return (
    sessionStorage.getItem(sessionTokenKey) ??
    localStorage.getItem(localTokenKey)
  );
}

export function getStoredUser(): UserProfile | null {
  const value =
    sessionStorage.getItem(sessionUserKey) ?? localStorage.getItem(localUserKey);

  if (!value) {
    return null;
  }

  try {
    return JSON.parse(value) as UserProfile;
  } catch {
    clearSession();
    return null;
  }
}

export function clearSession(): void {
  sessionStorage.removeItem(sessionTokenKey);
  sessionStorage.removeItem(sessionUserKey);
  localStorage.removeItem(localTokenKey);
  localStorage.removeItem(localUserKey);
}

export async function getCurrentUser(): Promise<UserProfile> {
  const token = getToken();
  if (!token) {
    throw new Error("Not authenticated");
  }

  return requestApi<UserProfile>("/me", {
    headers: {
      Authorization: `Bearer ${token}`,
    },
  });
}

