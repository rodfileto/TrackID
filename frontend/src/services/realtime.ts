import { apiBaseUrl, requestApi } from "./api";
import { getToken } from "./auth";

/** One message from the API's WebSocket (realtime package). "connected" comes from here, after
 * every successful connection: pushes may have been missed, so listeners should fetch again. */
export interface RealtimeEvent {
  topic?: string;
  type: string;
  data?: unknown;
}

type Listener = (event: RealtimeEvent) => void;

// Close codes from realtime/hub.go.
const closeTokenExpired = 4001;
const closeAccessRevoked = 4003;

const baseDelay = 1000;
const maxDelay = 30_000;
const tokenPollInterval = 5000;

const listeners = new Set<Listener>();
let socket: WebSocket | null = null;
let retryTimer: number | undefined;
let attempts = 0;
// Each start or stop bumps this, so work left over from an earlier connection can tell it is stale.
let generation = 0;

/** Follows the API's live events, sharing one socket across listeners. Returns the unsubscribe. */
export function subscribeRealtime(listener: Listener): () => void {
  listeners.add(listener);
  if (listeners.size === 1) void connect();
  return () => {
    listeners.delete(listener);
    if (listeners.size === 0) disconnect();
  };
}

function disconnect(): void {
  generation += 1;
  window.clearTimeout(retryTimer);
  socket?.close();
  socket = null;
  attempts = 0;
}

function emit(event: RealtimeEvent): void {
  listeners.forEach((listener) => listener(event));
}

function retryLater(): void {
  const delay = Math.min(maxDelay, baseDelay * 2 ** attempts) + Math.random() * baseDelay;
  attempts += 1;
  retryTimer = window.setTimeout(() => void connect(), delay);
}

/** After the token ended, the socket waits for a new one. Asking for a ticket now would open the
 * sign-in dialog on a tab nobody is using, so it leaves that to the user's next action. */
function waitForNewToken(expired: string): void {
  retryTimer = window.setTimeout(() => {
    const token = getToken();
    if (token && token !== expired) void connect();
    else waitForNewToken(expired);
  }, tokenPollInterval);
}

async function connect(): Promise<void> {
  generation += 1;
  const current = generation;
  const token = getToken();
  if (!token) return;

  let ticket: string;
  try {
    ({ ticket } = await requestApi<{ ticket: string }>("/realtime/ticket", {
      method: "POST",
      headers: { Authorization: `Bearer ${token}` },
    }));
  } catch {
    if (current === generation) retryLater();
    return;
  }
  if (current !== generation) return;

  const url = new URL(`${apiBaseUrl}/realtime`, window.location.href);
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  url.searchParams.set("ticket", ticket);

  const opened = new WebSocket(url);
  socket = opened;
  opened.onopen = () => {
    if (current !== generation) return;
    attempts = 0;
    emit({ type: "connected" });
  };
  opened.onmessage = (message) => {
    if (current === generation) emit(JSON.parse(message.data as string) as RealtimeEvent);
  };
  opened.onclose = (closed) => {
    if (current !== generation) return;
    socket = null;
    if (closed.code === closeAccessRevoked) return;
    if (closed.code === closeTokenExpired) waitForNewToken(token);
    else retryLater();
  };
}
