import { apiBaseUrl, apiFetch, requestApi } from "./api";
import { getToken } from "./auth";
import { appendOrigin, type MaterialOrigin } from "./collections";
import type { VideoLabFace } from "./videolab";

/** One face of a person in a video (video.Face): the same shape the video lab shows. */
export type VideoFace = VideoLabFace;

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
  /** Every face of the person, in time order, the best marked; absent for videos processed before
   * they were kept (process again to list them). */
  faces?: VideoFace[];
}

export interface CaseVideo {
  fileId: number;
  filename: string;
  sha256: string;
  contentType?: string;
  sizeBytes: number;
  requestId?: number;
  /** Or the scene collection that gathered it. */
  collectionId?: number;
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
export async function uploadCaseVideo(caseId: string, file: File, origin: MaterialOrigin): Promise<CaseVideo> {
  const form = new FormData();
  appendOrigin(form, origin);
  form.append("file", file);
  const response = await apiFetch(`${apiBaseUrl}/cases/${encodeURIComponent(caseId)}/videos`, {
    method: "POST",
    headers: authHeaders(),
    body: form,
  });
  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as { error?: string };
    // A refusal sent before the upload was read can reach the page as a bare 500 with no body
    // (the server closes the connection; a proxy reports that as an error of its own).
    throw new Error(body.error ?? "Could not upload the video (the server refused it or closed the connection)");
  }
  return (await response.json()) as CaseVideo;
}

/** A face to mark on a frame taken as an evidence (video.TraceBox). */
export interface TraceBox {
  box: [number, number, number, number];
  score: number;
}

/** Takes the frame at timestampMs as a new evidence, with each of faces marked on it as a trace. */
export async function extractVideoFrame(
  caseId: string,
  fileId: number,
  timestampMs: number,
  faces: TraceBox[],
): Promise<{ existing?: boolean; traces: unknown[] }> {
  return requestApi(`/cases/${encodeURIComponent(caseId)}/videos/${fileId}/frames`, {
    method: "POST",
    headers: authHeaders(),
    body: JSON.stringify({ timestampMs, faces }),
  });
}

/** A face found in a frame (video.FrameFace), with the person it matches when it is one. */
export interface FrameFace {
  box: [number, number, number, number];
  score: number;
  track?: number;
}

/** One frame of a case's video with the faces in it (video.Frame). */
export interface VideoFrame {
  timestampMs: number;
  frame?: number;
  width: number;
  height: number;
  /** A JPEG data: URI, for display. */
  image: string;
  faces: FrameFace[];
  /** False: vision is not configured, the faces are the tracks' at the nearest sampled time. */
  detected: boolean;
}

export async function getFrameFaces(caseId: string, fileId: number, timestampMs: number): Promise<VideoFrame> {
  return requestApi<VideoFrame>(`/cases/${encodeURIComponent(caseId)}/videos/${fileId}/frame-faces`, {
    method: "POST",
    headers: authHeaders(),
    body: JSON.stringify({ timestampMs: Math.round(timestampMs) }),
  });
}

/** Fetches a case's video to play it in the page, as an object URL the caller revokes. */
export async function getVideoObjectUrl(caseId: string, fileId: number, signal?: AbortSignal): Promise<string> {
  const response = await apiFetch(`${apiBaseUrl}/cases/${encodeURIComponent(caseId)}/videos/${fileId}/content`, {
    headers: authHeaders(),
    signal,
  });
  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(body.error ?? "Could not load the video");
  }
  return URL.createObjectURL(await response.blob());
}

/** Queues a case's video to be processed again. */
export async function reprocessVideo(caseId: string, fileId: number): Promise<void> {
  const response = await apiFetch(`${apiBaseUrl}/cases/${encodeURIComponent(caseId)}/videos/${fileId}/reprocess`, {
    method: "POST",
    headers: authHeaders(),
  });
  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(body.error ?? "Could not process the video again");
  }
}

/** A time in a video as HH:MM:SS.mmm. */
export function clock(ms: number): string {
  const pad = (n: number, w = 2) => String(Math.floor(n)).padStart(w, "0");
  return `${pad(ms / 3600000)}:${pad((ms / 60000) % 60)}:${pad((ms / 1000) % 60)}.${pad(ms % 1000, 3)}`;
}
