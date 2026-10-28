import { apiBaseUrl, requestApi } from "./api";

export interface UserProfile {
  id: number;
  first_name: string;
  last_name: string;
  staff_number: string;
  job_title: string;
  username: string;
  email: string;
  account_role: "SUPER_ADMIN" | "ADMIN" | "USER";
  /** Which biometrics the user may sign decisions on: either, both, or neither. */
  examines_fingerprint: boolean;
  examines_facial: boolean;
  /** Reads every case and person, and the intelligence pages (docs/access-profiles-plan.md). */
  intelligence_analyst: boolean;
  /** Distributes analyses, reassigns their steps, opens comparisons. */
  coordinator: boolean;
  is_active: boolean;
  /** When the user's picture was last set; absent while they have none. */
  avatar_updated_at?: string;
}

interface LoginResponse {
  token: string;
  user: UserProfile;
}

const sessionTokenKey = "trackid.session.token";
const localTokenKey = "trackid.local.token";
const sessionUserKey = "trackid.session.user";
const localUserKey = "trackid.local.user";

export async function login(
  staff_number: string,
  password: string,
  keepLoggedIn: boolean,
): Promise<UserProfile> {
  const response = await requestApi<LoginResponse>("/auth/login", {
    method: "POST",
    body: JSON.stringify({ staff_number, password }),
  });

  const tokenStorage = keepLoggedIn ? localStorage : sessionStorage;
  const userStorage = keepLoggedIn ? localStorage : sessionStorage;
  const tokenKey = keepLoggedIn ? localTokenKey : sessionTokenKey;
  const userKey = keepLoggedIn ? localUserKey : sessionUserKey;

  tokenStorage.setItem(tokenKey, response.token);
  userStorage.setItem(userKey, JSON.stringify(response.user));

  return response.user;
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

function bearer(): Record<string, string> {
  const token = getToken();
  return token ? { Authorization: `Bearer ${token}` } : {};
}

/** Replaces the stored profile, in whichever storage the session uses. */
export function storeUser(user: UserProfile): void {
  if (sessionStorage.getItem(sessionUserKey) !== null) sessionStorage.setItem(sessionUserKey, JSON.stringify(user));
  else localStorage.setItem(localUserKey, JSON.stringify(user));
}

/** What a user may change about themselves; username, staff number and email are fixed. */
export interface UserDetails {
  first_name: string;
  last_name: string;
  job_title: string;
}

export function updateMe(details: UserDetails): Promise<UserProfile> {
  return requestApi<UserProfile>("/me", { method: "PATCH", headers: bearer(), body: JSON.stringify(details) });
}

export async function changeMyPassword(currentPassword: string, newPassword: string): Promise<void> {
  const response = await fetch(`${apiBaseUrl}/me/password`, {
    method: "PUT",
    headers: { "Content-Type": "application/json", ...bearer() },
    body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }),
  });
  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(body.error ?? "Could not change the password");
  }
}

export async function uploadAvatar(file: File): Promise<UserProfile> {
  const form = new FormData();
  form.append("file", file);
  const response = await fetch(`${apiBaseUrl}/me/avatar`, { method: "PUT", headers: bearer(), body: form });
  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(body.error ?? "Could not store the picture");
  }
  return (await response.json()) as UserProfile;
}

export function removeAvatar(): Promise<UserProfile> {
  return requestApi<UserProfile>("/me/avatar", { method: "DELETE", headers: bearer() });
}

/** A user's picture as an object URL (the request needs the token, so an <img> cannot load it
 * directly). The caller revokes it. */
export async function fetchAvatar(userId: number, version: string): Promise<string> {
  const response = await fetch(`${apiBaseUrl}/users/${userId}/avatar?v=${encodeURIComponent(version)}`, { headers: bearer() });
  if (!response.ok) throw new Error("no picture");
  return URL.createObjectURL(await response.blob());
}
