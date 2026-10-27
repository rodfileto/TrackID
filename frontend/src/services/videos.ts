import { apiBaseUrl, requestApi } from "./api";
import { getToken } from "./auth";

/** One person videoproc followed through a video: the best face (time, box, crop) and its span. */
export interface VideoTrack {
  track: number;
  timestampMs: number;
  box: [number, number, number, number];
  blur: number;
  confidence: number;
  /** A small data: URI crop, for display only. */
  crop: string;
  detections: number;
  firstMs: number;
  lastMs: number;
}

export interface CaseVideo {
  fileId: number;
  filename: string;
  sha256: string;
  contentType?: string;
  sizeBytes: number;
  requestId?: number;
  status: "PENDING" | "PROCESSING" | "DONE" | "FAILED";
  durationSeconds?: number;
  frameRate?: number;
  tracks: VideoTrack[];
  error?: string;
  createdAt: string;
  processedAt?: string;
}

function authHeaders(): Record<string, string> {
  const token = getToken();
  return token ? { Authorization: `Bearer ${token}` } : {};
}

export async function listCaseVideos(caseId: string): Promise<CaseVideo[]> {
  return requestApi<CaseVideo[]>(`/cases/${encodeURIComponent(caseId)}/videos`, { headers: authHeaders() });
}

/** Uploads a video sent in; the worker then finds its tracks. */
export async function uploadCaseVideo(caseId: string, file: File): Promise<CaseVideo> {
  const form = new FormData();
  form.append("file", file);
  const response = await fetch(`${apiBaseUrl}/cases/${encodeURIComponent(caseId)}/videos`, {
    method: "POST",
    headers: authHeaders(),
    body: form,
  });
  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(body.error ?? "Could not upload the video");
  }
  return (await response.json()) as CaseVideo;
}

/** Takes the frame at timestampMs as a new evidence, with the face in box marked on it. */
export async function extractVideoFrame(caseId: string, fileId: number, timestampMs: number, box?: VideoTrack["box"]): Promise<void> {
  await requestApi(`/cases/${encodeURIComponent(caseId)}/videos/${fileId}/frames`, {
    method: "POST",
    headers: authHeaders(),
    body: JSON.stringify({ timestampMs, box }),
  });
}

/** A time in a video as HH:MM:SS.mmm. */
export function clock(ms: number): string {
  const pad = (n: number, w = 2) => String(Math.floor(n)).padStart(w, "0");
  return `${pad(ms / 3600000)}:${pad((ms / 60000) % 60)}:${pad((ms / 1000) % 60)}.${pad(ms % 1000, 3)}`;
}
