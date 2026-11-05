import { apiBaseUrl, apiFetch } from "./api";
import { getToken } from "./auth";

// The video lab's API (api/videolab_handler.go, video/lab.go): send any video, trackid-vision finds
// the faces and groups them by person, the best face of each marked. Nothing is stored; the video
// stays in a temporary file on the server so full frames can be extracted.

export interface VideoLabInfo {
  enabled: boolean;
  defaultInterval: number;
  minInterval: number;
  maxInterval: number;
  defaultMinBlur: number;
  maxBytes: number;
  software: string;
}

export interface VideoLabFace {
  timestampMs: number;
  /** The video frame number at that time, when the frame rate is known. */
  frame?: number;
  sample: number;
  box: [number, number, number, number];
  confidence: number;
  blur: number;
  embedded: boolean;
  best: boolean;
  crop: string;
}

export interface VideoLabPerson {
  track: number;
  best: VideoLabFace;
  faces: VideoLabFace[];
  firstMs: number;
  lastMs: number;
}

export interface VideoLabJob {
  id: string;
  filename: string;
  sha256: string;
  sizeBytes: number;
  options: { intervalSeconds: number; minBlur: number };
  status: "PENDING" | "PROCESSING" | "DONE" | "FAILED";
  frames: number;
  expectedFrames: number;
  durationSeconds?: number;
  frameRate?: number;
  persons: VideoLabPerson[];
  error?: string;
  createdAt: string;
  finishedAt?: string;
  software: string;
}

/** The video is gone from the server (expired or removed): send it again. */
export class VideoLabExpired extends Error {}

function authHeaders(): Record<string, string> {
  const token = getToken();
  return token ? { Authorization: `Bearer ${token}` } : {};
}

async function fail(response: Response, fallback: string): Promise<never> {
  const body = (await response.json().catch(() => ({}))) as { error?: string };
  if (response.status === 410) throw new VideoLabExpired(body.error ?? fallback);
  throw new Error(body.error ?? fallback);
}

export async function getVideoLabInfo(): Promise<VideoLabInfo> {
  const response = await apiFetch(`${apiBaseUrl}/videolab`, { headers: authHeaders() });
  if (!response.ok) return fail(response, "The request could not be completed");
  return (await response.json()) as VideoLabInfo;
}

export async function uploadVideoLab(file: File, intervalSeconds: number, minBlur: number): Promise<VideoLabJob> {
  const form = new FormData();
  // Options before the file: the server reads them before streaming the video to disk.
  form.append("intervalSeconds", String(intervalSeconds));
  form.append("minBlur", String(minBlur));
  form.append("file", file);
  const response = await apiFetch(`${apiBaseUrl}/videolab/jobs`, { method: "POST", headers: authHeaders(), body: form });
  if (!response.ok) return fail(response, "Could not send the video");
  return (await response.json()) as VideoLabJob;
}

export async function getVideoLabJob(id: string, signal?: AbortSignal): Promise<VideoLabJob> {
  const response = await apiFetch(`${apiBaseUrl}/videolab/jobs/${encodeURIComponent(id)}`, { headers: authHeaders(), signal });
  if (!response.ok) return fail(response, "The request could not be completed");
  return (await response.json()) as VideoLabJob;
}

export async function deleteVideoLabJob(id: string): Promise<void> {
  const response = await apiFetch(`${apiBaseUrl}/videolab/jobs/${encodeURIComponent(id)}`, { method: "DELETE", headers: authHeaders() });
  if (!response.ok && response.status !== 410) return fail(response, "Could not remove the video");
}

/** The full frame at timestampMs, extracted on the server from the original, as a blob. */
export async function getVideoLabFrame(id: string, timestampMs: number): Promise<Blob> {
  const response = await apiFetch(`${apiBaseUrl}/videolab/jobs/${encodeURIComponent(id)}/frame?ms=${Math.round(timestampMs)}`, {
    headers: authHeaders(),
  });
  if (!response.ok) return fail(response, "Could not extract the frame");
  return response.blob();
}

/** HH:MM:SS.mmm */
export function clock(ms: number): string {
  const total = Math.max(0, Math.round(ms));
  const h = Math.floor(total / 3_600_000);
  const m = Math.floor(total / 60_000) % 60;
  const s = Math.floor(total / 1000) % 60;
  const pad = (n: number, w = 2) => String(n).padStart(w, "0");
  return `${pad(h)}:${pad(m)}:${pad(s)}.${pad(total % 1000, 3)}`;
}
