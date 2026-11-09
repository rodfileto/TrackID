// What the app remembers about each entry of this tab's history, by router location key: the path
// it was reached from and how far it was scrolled. sessionStorage, so a reload keeps it and a new
// tab (which has no history to go back through) starts empty.
interface TrailEntry {
  /** Path and query of the page this entry was reached from, inside the app. */
  from?: string;
  scrollY?: number;
}

const storageKey = "trackid.navigation";
const maxEntries = 200;

function load(): Map<string, TrailEntry> {
  try {
    return new Map(JSON.parse(sessionStorage.getItem(storageKey) ?? "[]") as [string, TrailEntry][]);
  } catch {
    return new Map();
  }
}

const trail = load();

export function saveTrail(): void {
  try {
    sessionStorage.setItem(storageKey, JSON.stringify([...trail].slice(-maxEntries)));
  } catch {
    // Only conveniences are lost: back links fall back to their list, pages open at the top.
  }
}

function update(key: string, patch: TrailEntry): void {
  // Re-inserted so the entries saveTrail keeps are the most recently used.
  const entry = { ...trail.get(key), ...patch };
  trail.delete(key);
  trail.set(key, entry);
}

const listeners = new Set<() => void>();

/** For useSyncExternalStore: the tracker records an entry after the page has rendered. */
export function subscribeToTrail(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

export function recordFrom(key: string, from: string | undefined): void {
  update(key, { from });
  listeners.forEach((listener) => listener());
}

export function recordScroll(key: string, scrollY: number): void {
  update(key, { scrollY });
}

export function cameFrom(key: string): string | undefined {
  return trail.get(key)?.from;
}

export function savedScroll(key: string): number | undefined {
  return trail.get(key)?.scrollY;
}
