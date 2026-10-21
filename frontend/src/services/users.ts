import { requestApi } from "./api";
import { getToken, type UserProfile } from "./auth";

export type AccountRole = UserProfile["account_role"];

/** The part of an account an admin changes: its role, capabilities and active flag. */
export type UserAccess = Pick<
  UserProfile,
  "account_role" | "examines_facial" | "examines_fingerprint" | "intelligence_analyst" | "coordinator" | "is_active"
>;

export interface NewUser extends Omit<UserAccess, "is_active"> {
  first_name: string;
  last_name: string;
  staff_number: string;
  job_title: string;
  username: string;
  password: string;
}

/** The shortest password the server accepts (auth.MinPasswordLength). */
export const minPasswordLength = 8;

function authHeaders(): Record<string, string> {
  const token = getToken();
  return token ? { Authorization: `Bearer ${token}` } : {};
}

/** Admin: every account, deactivated ones included. */
export async function listUsers(): Promise<UserProfile[]> {
  return requestApi<UserProfile[]>("/admin/users", { headers: authHeaders() });
}

export async function createUser(user: NewUser): Promise<UserProfile> {
  return requestApi<UserProfile>("/admin/users", {
    method: "POST",
    headers: authHeaders(),
    body: JSON.stringify(user),
  });
}

/** Admin: change some of an account's access; fields left out keep their value. */
export async function updateUserAccess(userId: number, access: Partial<UserAccess>): Promise<UserProfile> {
  return requestApi<UserProfile>(`/admin/users/${userId}`, {
    method: "PATCH",
    headers: authHeaders(),
    body: JSON.stringify(access),
  });
}

/** Admin: replace an account's password. */
export async function setUserPassword(userId: number, password: string): Promise<UserProfile> {
  return requestApi<UserProfile>(`/admin/users/${userId}/password`, {
    method: "PUT",
    headers: authHeaders(),
    body: JSON.stringify({ password }),
  });
}

/** A random password a person can type: 10 characters with no look-alikes. */
export function generatePassword(): string {
  const alphabet = "abcdefghjkmnpqrstuvwxyz23456789";
  const bytes = new Uint32Array(10);
  crypto.getRandomValues(bytes);
  return Array.from(bytes, (b) => alphabet[b % alphabet.length]).join("");
}
