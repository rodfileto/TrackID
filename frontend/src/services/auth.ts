import { apiBaseUrl, apiFetch, requestApi, setSessionHandlers } from "./api";

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

// A session without "keep me logged in" lives in sessionStorage, which each tab has its own copy
// of, so a link opened in a new tab would start signed out. The tabs share it over a
// BroadcastChannel instead: a new tab asks, and any tab holding the session answers. It still ends
// when the browser closes (or the last tab holding it does), which is what the choice promises,
// and the token never goes anywhere but this origin's other tabs.
/** Why a session ended, when the user should be told. */
export type SessionEndReason = "deactivated";

type SessionMessage =
  | { type: "request" }
  | { type: "grant"; token: string; user: string | null }
  | { type: "renewed"; token: string; userId: number }
  | { type: "signed-out"; reason?: SessionEndReason };

const sessionChannel = typeof BroadcastChannel === "undefined" ? null : new BroadcastChannel("trackid.session");
const sessionEndListeners = new Set<(reason?: SessionEndReason) => void>();

sessionChannel?.addEventListener("message", (event: MessageEvent<SessionMessage>) => {
  if (event.data.type === "request") {
    const token = sessionStorage.getItem(sessionTokenKey);
    if (token) sessionChannel.postMessage({ type: "grant", token, user: sessionStorage.getItem(sessionUserKey) } satisfies SessionMessage);
  } else if (event.data.type === "renewed") {
    if (getStoredUser()?.id !== event.data.userId) return;
    if (sessionStorage.getItem(sessionTokenKey) !== null) sessionStorage.setItem(sessionTokenKey, event.data.token);
    else localStorage.setItem(localTokenKey, event.data.token);
    settleExpiry(event.data.token);
  } else if (event.data.type === "signed-out") {
    clearSession();
    abandonExpiry();
    const { reason } = event.data;
    sessionEndListeners.forEach((listener) => listener(reason));
  }
});

/** How long a tab with no session waits for another tab to offer one before showing sign-in. */
const sessionRequestTimeoutMs = 400;

/** Asks the other tabs for their session and stores it here. Resolves to whether this tab now has one. */
export function requestSessionFromOtherTabs(): Promise<boolean> {
  const channel = sessionChannel;
  if (!channel) return Promise.resolve(false);
  return new Promise((resolve) => {
    const finish = (granted: boolean) => {
      channel.removeEventListener("message", onMessage);
      clearTimeout(timer);
      resolve(granted);
    };
    const onMessage = (event: MessageEvent<SessionMessage>) => {
      if (event.data.type !== "grant") return;
      sessionStorage.setItem(sessionTokenKey, event.data.token);
      if (event.data.user) sessionStorage.setItem(sessionUserKey, event.data.user);
      finish(true);
    };
    const timer = setTimeout(() => finish(false), sessionRequestTimeoutMs);
    channel.addEventListener("message", onMessage);
    channel.postMessage({ type: "request" } satisfies SessionMessage);
  });
}

/** Ends the session in this tab and every other one. */
export function signOut(): void {
  clearSession();
  abandonExpiry();
  sessionChannel?.postMessage({ type: "signed-out" } satisfies SessionMessage);
}

/** Calls back when the session ends other than by this tab's own sign-out. Returns the unsubscribe. */
export function onSessionEnded(listener: (reason?: SessionEndReason) => void): () => void {
  sessionEndListeners.add(listener);
  return () => sessionEndListeners.delete(listener);
}

// A token runs out on its own schedule, often mid-work. Sending the user to the sign-in page would
// throw away the page and whatever is unsaved on it, so the requests wait (apiFetch), the page stays
// mounted under SessionExpiredDialog, and signing in again lets them go on.
let waitingRequests: { resolve: (token: string) => void; reject: (reason: Error) => void }[] = [];
const expiryListeners = new Set<() => void>();

function settleExpiry(token: string): void {
  const waiting = waitingRequests;
  waitingRequests = [];
  waiting.forEach((request) => request.resolve(token));
  expiryListeners.forEach((listener) => listener());
}

function abandonExpiry(): void {
  const waiting = waitingRequests;
  waitingRequests = [];
  waiting.forEach((request) => request.reject(new Error("Not authenticated")));
  expiryListeners.forEach((listener) => listener());
}

setSessionHandlers({
  renew(refused) {
    const current = getToken();
    // The session changed while the request was out, or was signed out of.
    if (current && current !== refused) return Promise.resolve(current);
    if (!current) return Promise.reject(new Error("Not authenticated"));
    // Nothing says whose session this is, so it cannot be renewed in place.
    if (!getStoredUser()) {
      clearSession();
      sessionEndListeners.forEach((listener) => listener());
      return Promise.reject(new Error("Not authenticated"));
    }
    return new Promise((resolve, reject) => {
      waitingRequests.push({ resolve, reject });
      if (waitingRequests.length === 1) expiryListeners.forEach((listener) => listener());
    });
  },
  deactivated() {
    // Several requests can be refused at once: the first one ends it.
    if (!getToken()) return;
    const reason: SessionEndReason = "deactivated";
    clearSession();
    abandonExpiry();
    sessionChannel?.postMessage({ type: "signed-out", reason } satisfies SessionMessage);
    sessionEndListeners.forEach((listener) => listener(reason));
  },
});

export function isSessionExpired(): boolean {
  return waitingRequests.length > 0;
}

export function subscribeToSessionExpiry(listener: () => void): () => void {
  expiryListeners.add(listener);
  return () => expiryListeners.delete(listener);
}

/** Signs the stored user in again, in the storage their session already uses, and lets the waiting
 * requests go on here and in every other tab of theirs. */
export async function signInAgain(password: string): Promise<void> {
  const user = getStoredUser();
  if (!user) throw new Error("Not authenticated");
  const keepLoggedIn = sessionStorage.getItem(sessionTokenKey) === null;
  await login(user.staff_number, password, keepLoggedIn);
  const token = getToken();
  if (!token) throw new Error("Not authenticated");
  sessionChannel?.postMessage({ type: "renewed", token, userId: user.id } satisfies SessionMessage);
  settleExpiry(token);
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
  const response = await apiFetch(`${apiBaseUrl}/me/password`, {
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
  const response = await apiFetch(`${apiBaseUrl}/me/avatar`, { method: "PUT", headers: bearer(), body: form });
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
  const response = await apiFetch(`${apiBaseUrl}/users/${userId}/avatar?v=${encodeURIComponent(version)}`, { headers: bearer() });
  if (!response.ok) throw new Error("no picture");
  return URL.createObjectURL(await response.blob());
}
