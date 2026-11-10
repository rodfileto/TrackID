import { requestApi } from "./api";
import { getToken } from "./auth";

/** What is followed: a person by person_id, or an AUTOMATIC cluster by id. Following a REVIEWED cluster
 * follows the AUTOMATIC cluster it sits inside, which subject then names. */
export interface FollowSubject {
  kind: "PERSON" | "CLUSTER";
  id: string;
}

export interface FollowState {
  subject: FollowSubject;
  following: boolean;
}

function headers(): Record<string, string> {
  const token = getToken();
  return { "Content-Type": "application/json", ...(token ? { Authorization: `Bearer ${token}` } : {}) };
}

function path(kind: FollowSubject["kind"], id: string | number): string {
  return `/follows/${kind.toLowerCase()}/${encodeURIComponent(String(id))}`;
}

export function getFollow(kind: FollowSubject["kind"], id: string | number): Promise<FollowState> {
  return requestApi<FollowState>(path(kind, id), { headers: headers() });
}

export function setFollow(kind: FollowSubject["kind"], id: string | number, following: boolean): Promise<FollowState> {
  return requestApi<FollowState>(path(kind, id), { method: following ? "PUT" : "DELETE", headers: headers() });
}
