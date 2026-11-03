import { apiBaseUrl, requestApi } from "./api";
import { getToken } from "./auth";

// The image lab's API (api/imageproc_handler.go): fingerprint processing tried on any image,
// nothing stored. The recipe mirrors imageproc.Recipe.

export type PpiMethod = "CALIBRATED" | "FILE" | "ESTIMATED" | "ASSUMED";
export type Channel = "luminance" | "red" | "green" | "blue";
export type Interpolation = "nearest" | "bilinear" | "bicubic";

export interface Rect {
  x1: number;
  y1: number;
  x2: number;
  y2: number;
}

export interface Recipe {
  version?: number;
  source: {
    ppi?: number;
    method: PpiMethod;
    calibration?: Rect & { mm: number };
    ridgePeriodPx?: number;
    ridgeDistanceMm?: number;
  };
  channel: Channel;
  crop?: Rect;
  rotate: number;
  mirror: boolean;
  interpolation: Interpolation;
  flatten?: { sigmaMm: number };
  levels?: { inBlack: number; inWhite: number; gamma: number; outBlack: number; outWhite: number };
  clahe?: { tiles: number; clipLimit: number };
  gabor?: { orientations: number };
  invert: boolean;
}

export interface LabInfo {
  outputPpi: number;
  recipeVersion: number;
  defaultRidgeDistanceMm: number;
  compare: boolean;
  matchThreshold: number;
  software: string;
}

export interface LabSource {
  id: string;
  filename: string;
  format: string;
  width: number;
  height: number;
  sha256: string;
  orientation: number;
  filePpi: number;
}

export interface LabRender {
  image: string;
  width: number;
  height: number;
  sha256: string;
  histogram: number[];
  transform: { A: number; B: number; C: number; D: number; E: number; F: number };
  recipe: Recipe;
  record: string;
  scale: number;
  gaborPeriodPx: number;
}

export interface LabComparison {
  score: number;
  threshold: number;
  referenceImage: string;
  referenceWidth: number;
  referenceHeight: number;
}

/** The original is no longer in the server's memory; upload it again and retry. */
export class SourceExpiredError extends Error {}

function authHeaders(): Record<string, string> {
  const token = getToken();
  return token ? { Authorization: `Bearer ${token}` } : {};
}

async function post<T>(path: string, body: unknown, signal?: AbortSignal): Promise<T> {
  const response = await fetch(`${apiBaseUrl}${path}`, {
    method: "POST",
    headers: { "Content-Type": "application/json", ...authHeaders() },
    body: JSON.stringify(body),
    signal,
  });
  if (!response.ok) {
    const payload = (await response.json().catch(() => ({}))) as { error?: string; code?: string };
    if (payload.code === "source_expired") throw new SourceExpiredError(payload.error);
    throw new Error(payload.error ?? "The request could not be completed");
  }
  return (await response.json()) as T;
}

export function getLabInfo(): Promise<LabInfo> {
  return requestApi<LabInfo>("/imageproc", { headers: authHeaders() });
}

export async function uploadLabSource(file: File): Promise<LabSource> {
  const form = new FormData();
  form.append("file", file);
  const response = await fetch(`${apiBaseUrl}/imageproc/sources`, { method: "POST", headers: authHeaders(), body: form });
  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(body.error ?? `Could not read ${file.name}`);
  }
  return (await response.json()) as LabSource;
}

export function estimateRidgePeriod(sourceId: string, crop: Rect | undefined, channel: Channel) {
  return post<{ periodPx: number; confidence: number }>("/imageproc/estimate", { sourceId, crop, channel });
}

export function renderLab(sourceId: string, recipe: Recipe, signal?: AbortSignal) {
  return post<LabRender>("/imageproc/render", { sourceId, recipe }, signal);
}

export function compareLab(sourceId: string, recipe: Recipe, referenceId: string, referenceRecipe: Recipe) {
  return post<LabComparison>("/imageproc/compare", { sourceId, recipe, referenceId, referenceRecipe });
}
