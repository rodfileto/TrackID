import { requestApi } from "./api";

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
