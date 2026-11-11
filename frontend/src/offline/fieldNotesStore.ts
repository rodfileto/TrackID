// Notes written at a scene, kept in the browser until they are synced (IndexedDB;
// docs/scene-examination-plan.md). A note and each of its audios carry an id made here, so
// sending one twice leaves one on the server. Nothing here needs a network.

export interface LocalAudio {
  clientId: string;
  title: string;
  blob: Blob;
  recordedAt: string; // ISO
  synced: boolean;
}

export interface LocalNote {
  clientId: string;
  placeType: string;
  placeName: string;
  lon: number | null;
  lat: number | null;
  address: string;
  neighbourhood: string;
  calledAt: string; // ISO, or ""
  examinedAt: string;
  occurredAt: string;
  team: number[];
  otherOfficers: string;
  notes: string;
  writtenAt: string; // ISO
  audios: LocalAudio[];
  /** Whether the note as it is now is on the server. An edit sets it false again. */
  synced: boolean;
  /** The server's id, once synced. */
  serverId?: number;
  /** Why the last sync failed, when it did. */
  error?: string;
}

/** A user who can be named in a note's team, kept for when there is no signal. */
export interface TeamUser {
  id: number;
  username: string;
  name: string;
}

const DB_NAME = "trackid-field";
const NOTES = "notes";
const KEPT = "kept";

function open(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const request = indexedDB.open(DB_NAME, 1);
    request.onupgradeneeded = () => {
      const db = request.result;
      if (!db.objectStoreNames.contains(NOTES)) db.createObjectStore(NOTES, { keyPath: "clientId" });
      if (!db.objectStoreNames.contains(KEPT)) db.createObjectStore(KEPT);
    };
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error);
  });
}

async function run<T>(store: string, mode: IDBTransactionMode, work: (s: IDBObjectStore) => IDBRequest<T>): Promise<T> {
  const db = await open();
  try {
    return await new Promise<T>((resolve, reject) => {
      const tx = db.transaction(store, mode);
      const request = work(tx.objectStore(store));
      tx.oncomplete = () => resolve(request.result);
      tx.onerror = () => reject(tx.error);
      tx.onabort = () => reject(tx.error);
    });
  } finally {
    db.close();
  }
}

/** A new id for a note or an audio. */
export function newClientId(): string {
  if (typeof crypto.randomUUID === "function") return crypto.randomUUID();
  // An older browser, or a page not served securely: the same shape from random bytes.
  const b = crypto.getRandomValues(new Uint8Array(16));
  b[6] = (b[6] & 0x0f) | 0x40;
  b[8] = (b[8] & 0x3f) | 0x80;
  const h = [...b].map((x) => x.toString(16).padStart(2, "0")).join("");
  return `${h.slice(0, 8)}-${h.slice(8, 12)}-${h.slice(12, 16)}-${h.slice(16, 20)}-${h.slice(20)}`;
}

/** The device's notes, newest first. */
export async function listLocalNotes(): Promise<LocalNote[]> {
  const notes = await run<LocalNote[]>(NOTES, "readonly", (s) => s.getAll());
  return notes.sort((a, b) => b.writtenAt.localeCompare(a.writtenAt));
}

export function getLocalNote(clientId: string): Promise<LocalNote | undefined> {
  return run<LocalNote | undefined>(NOTES, "readonly", (s) => s.get(clientId));
}

export async function saveLocalNote(note: LocalNote): Promise<void> {
  await run(NOTES, "readwrite", (s) => s.put(note));
}

export async function deleteLocalNote(clientId: string): Promise<void> {
  await run(NOTES, "readwrite", (s) => s.delete(clientId));
}

/** The team list as last fetched, for the form with no signal. */
export async function keptTeam(): Promise<TeamUser[]> {
  return (await run<TeamUser[] | undefined>(KEPT, "readonly", (s) => s.get("team"))) ?? [];
}

export async function keepTeam(team: TeamUser[]): Promise<void> {
  await run(KEPT, "readwrite", (s) => s.put(team, "team"));
}

/** Asks the browser not to clear this site's storage when space runs short. Best effort. */
export async function askToPersist(): Promise<void> {
  try {
    await navigator.storage?.persist?.();
  } catch {
    // Not supported: the notes are still kept, only less protected.
  }
}
