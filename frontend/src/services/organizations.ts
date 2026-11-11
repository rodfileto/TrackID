import { requestApi } from "./api";
import { getToken } from "./auth";

/** Organizations, their units and where each user works (docs/organizations-plan.md). */

export type OrganizationKind = "FEDERAL" | "STATE";

export interface Organization {
  id: number;
  acronym: string;
  name: string;
  kind: OrganizationKind;
  uf?: string;
  active: boolean;
  unit_count: number;
}

export interface OrganizationInput {
  acronym: string;
  name: string;
  kind: OrganizationKind;
  uf: string;
  active: boolean;
}

export interface Unit {
  id: number;
  organization_id: number;
  parent_id: number | null;
  acronym: string;
  name: string;
  uf?: string;
  municipality_code?: string;
  receives_new_cases: boolean;
  active: boolean;
  /** The laudo header's lines; empty takes the unit above's. */
  laudo_header: string[];
  /** How a laudo's preâmbulo names the unit; empty is its name. */
  laudo_name: string;
  current_members: number;
}

export interface UnitInput {
  parent_id: number | null;
  acronym: string;
  name: string;
  uf: string;
  municipality_code: string;
  laudo_header: string[];
  laudo_name: string;
  active: boolean;
  receives_new_cases: boolean;
}

/** POSTING is lotação, MISSION is missão. */
export type AssignmentKind = "POSTING" | "MISSION";

export interface Assignment {
  id: number;
  user_id: number;
  unit_id: number;
  kind: AssignmentKind;
  /** YYYY-MM-DD; ends_on is the last day, absent when open-ended. */
  starts_on: string;
  ends_on?: string;
  note: string;
}

export interface AssignmentInput {
  user_id: number;
  unit_id: number;
  kind: AssignmentKind;
  starts_on: string;
  ends_on: string;
  note: string;
}

function authHeaders(): Record<string, string> {
  const token = getToken();
  return token ? { Authorization: `Bearer ${token}` } : {};
}

function send<T>(method: string, path: string, body?: unknown): Promise<T> {
  return requestApi<T>(path, {
    method,
    headers: authHeaders(),
    body: body === undefined ? undefined : JSON.stringify(body),
  });
}

export const listOrganizations = () => send<Organization[]>("GET", "/admin/organizations");
export const createOrganization = (o: OrganizationInput) => send<{ id: number }>("POST", "/admin/organizations", o);
export const updateOrganization = (id: number, o: OrganizationInput) => send<void>("PUT", `/admin/organizations/${id}`, o);
export const deleteOrganization = (id: number) => send<void>("DELETE", `/admin/organizations/${id}`);

export const listUnits = () => send<Unit[]>("GET", "/admin/units");
export const createUnit = (u: UnitInput) => send<{ id: number }>("POST", "/admin/units", u);
export const updateUnit = (id: number, u: UnitInput) => send<void>("PUT", `/admin/units/${id}`, u);
export const deleteUnit = (id: number) => send<void>("DELETE", `/admin/units/${id}`);

export const listAssignments = () => send<Assignment[]>("GET", "/admin/unit-assignments");
export const createAssignment = (a: AssignmentInput) => send<{ id: number }>("POST", "/admin/unit-assignments", a);
export const updateAssignment = (id: number, a: AssignmentInput) => send<void>("PUT", `/admin/unit-assignments/${id}`, a);
export const deleteAssignment = (id: number) => send<void>("DELETE", `/admin/unit-assignments/${id}`);

/** Today in São Paulo, YYYY-MM-DD. */
export function today(): string {
  return new Intl.DateTimeFormat("en-CA", { timeZone: "America/Sao_Paulo" }).format(new Date());
}

/** Whether an assignment covers a day. */
export function covers(a: Assignment, day: string): boolean {
  return a.starts_on <= day && (!a.ends_on || a.ends_on >= day);
}

/** Where a user works on a day: the mission covering it, else the posting (organizations.UnitOn). */
export function placementOn(assignments: Assignment[], userId: number, day: string): Assignment | undefined {
  const mine = assignments.filter((a) => a.user_id === userId && covers(a, day));
  return mine.find((a) => a.kind === "MISSION") ?? mine.find((a) => a.kind === "POSTING");
}

/** The laudo header a unit gives: its own lines, or its nearest ancestor's, or its organization's name. */
export function effectiveHeader(unit: Unit, byId: Map<number, Unit>, organizationName: string): { lines: string[]; from?: Unit } {
  for (let u: Unit | undefined = unit, depth = 0; u && depth < 64; u = u.parent_id ? byId.get(u.parent_id) : undefined, depth++) {
    if (u.laudo_header.length > 0) return { lines: u.laudo_header, from: u };
  }
  return { lines: [organizationName] };
}

/** "SR/PF/MG › DREX › NID": a unit's place in its tree, root left out. */
export function unitPath(unit: Unit, byId: Map<number, Unit>): string {
  const parts: string[] = [];
  for (let u: Unit | undefined = unit, depth = 0; u && u.parent_id !== null && depth < 64; u = byId.get(u.parent_id), depth++) {
    parts.unshift(u.acronym);
  }
  return parts.length ? parts.join(" › ") : unit.acronym;
}
