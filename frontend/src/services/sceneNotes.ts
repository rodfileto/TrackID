import { apiBaseUrl, apiFetch, requestApi, type ErrorResponse } from "./api";
import { getToken } from "./auth";
import type { LocalNote, TeamUser } from "../offline/fieldNotesStore";

// Notes written at a scene, once synced (docs/scene-examination-plan.md): drafts on the server,
// each to become a scene collection.

export interface SceneNoteAudio {
  id: number;
  clientId: string;
  title?: string;
  contentType: string;
  sizeBytes: number;
  sha256: string;
  recordedAt?: string;
}

export interface SceneNote {
  id: number;
  clientId: string;
  authorId: number;
  author: string;
  placeType?: string;
  placeName?: string;
  lon?: number;
  lat?: number;
  address?: string;
  neighbourhood?: string;
  geocodeSource?: string;
  geocodeFlags?: string[];
  calledAt?: string;
  examinedAt?: string;
  occurredAt?: string;
  team: TeamUser[];
  otherOfficers?: string;
  notes?: string;
  writtenAt: string;
  syncedAt: string;
  /** Set once the note became a collection. */
  collectionId?: number;
  audioCount: number;
  audios?: SceneNoteAudio[];
}

/** The sync needs a session: the notes stay on the device until the user signs in. */
export class SignInNeeded extends Error {
  constructor() {
    super("sign in to sync");
  }
}

function authHeaders(): Record<string, string> {
  return { Authorization: `Bearer ${getToken() ?? ""}` };
}

// The field page works with no session, so it calls the API without the wrapper that waits for a
// new sign-in: a refused token is told to the page instead.
async function send<T>(path: string, init: RequestInit): Promise<T> {
  if (!getToken()) throw new SignInNeeded();
  const response = await fetch(`${apiBaseUrl}${path}`, { ...init, headers: { ...authHeaders(), ...init.headers } });
  if (response.status === 401) throw new SignInNeeded();
  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as ErrorResponse;
    throw new Error(body.error ?? `HTTP ${response.status}`);
  }
  return (await response.json()) as T;
}

const iso = (value: string) => (value ? value : undefined);

/** Sends a device's note, new or again; the server keeps one per clientId. */
export function syncNote(note: LocalNote): Promise<SceneNote> {
  return send<SceneNote>("/scene-notes", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      clientId: note.clientId,
      placeType: note.placeType || undefined,
      placeName: note.placeName || undefined,
      lon: note.lon ?? undefined,
      lat: note.lat ?? undefined,
      address: note.address || undefined,
      neighbourhood: note.neighbourhood || undefined,
      calledAt: iso(note.calledAt),
      examinedAt: iso(note.examinedAt),
      occurredAt: iso(note.occurredAt),
      team: note.team,
      otherOfficers: note.otherOfficers || undefined,
      notes: note.notes || undefined,
      writtenAt: note.writtenAt,
    }),
  });
}

/** Sends one audio of a synced note; the server keeps one per clientId. */
export function syncAudio(noteId: number, audio: { clientId: string; title: string; blob: Blob; recordedAt: string }): Promise<SceneNoteAudio> {
  const form = new FormData();
  const extension = audio.blob.type.includes("mp4") ? "m4a" : audio.blob.type.includes("ogg") ? "ogg" : "webm";
  form.append("file", audio.blob, `${audio.clientId}.${extension}`);
  form.append("clientId", audio.clientId);
  if (audio.title) form.append("title", audio.title);
  form.append("recordedAt", audio.recordedAt);
  return send<SceneNoteAudio>(`/scene-notes/${noteId}/audios`, { method: "POST", body: form });
}

/** Who can be named in a team: the active fingerprint examiners. */
export async function fetchTeam(): Promise<TeamUser[]> {
  return (await send<{ items: TeamUser[] }>("/scene-notes/team", { method: "GET" })).items;
}

/** The caller's synced notes; everyone's for a coordinator who asks. */
export async function listSceneNotes(everyone = false, converted = false): Promise<SceneNote[]> {
  return (await send<{ items: SceneNote[] }>(`/scene-notes?everyone=${everyone}&converted=${converted}`, { method: "GET" })).items;
}

export function getSceneNote(id: number): Promise<SceneNote> {
  return requestApi<SceneNote>(`/scene-notes/${id}`, { headers: authHeaders() });
}

export async function discardSceneNote(id: number): Promise<void> {
  const response = await apiFetch(`${apiBaseUrl}/scene-notes/${id}`, { method: "DELETE", headers: authHeaders() });
  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as ErrorResponse;
    throw new Error(body.error ?? `HTTP ${response.status}`);
  }
}

/** An audio as an object URL, to play; the caller revokes it. */
export async function sceneNoteAudioUrl(audioId: number): Promise<string> {
  const response = await apiFetch(`${apiBaseUrl}/scene-notes/audios/${audioId}`, { headers: authHeaders() });
  if (!response.ok) throw new Error(`HTTP ${response.status}`);
  return URL.createObjectURL(await response.blob());
}

/** An audio note of a case's collection as an object URL; the caller revokes it. */
export async function collectionNoteUrl(caseId: string, fileId: number): Promise<string> {
  const response = await apiFetch(`${apiBaseUrl}/cases/${encodeURIComponent(caseId)}/collection-notes/${fileId}`, { headers: authHeaders() });
  if (!response.ok) throw new Error(`HTTP ${response.status}`);
  return URL.createObjectURL(await response.blob());
}
