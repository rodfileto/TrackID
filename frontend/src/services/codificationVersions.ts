import { apiBaseUrl } from "./api";
import { getToken } from "./auth";

// Codification versions of a face (cases.CodificationVersion, api/codification_versions_handler.go):
// the original and versions rendered from it with a recorded recipe; the matcher uses the one
// chosen for recognition.

/** imageproc.FaceRecipe as the versions editor sets it (the crop is always the server's). */
export interface FaceRecipe {
  version?: number;
  rotate: number;
  mirror: boolean;
  scale: number;
  interpolation?: string;
  grey?: string;
  brightnessContrast?: { brightness: number; contrast: number };
  levels?: { inBlack: number; inWhite: number; gamma: number; outBlack: number; outWhite: number };
  colorBalance?: { red: number; green: number; blue: number };
  crop?: { x1: number; y1: number; x2: number; y2: number };
}

export interface CodificationVersion {
  id: number;
  sequence: number;
  label?: string;
  recipe?: FaceRecipe;
  record?: string;
  hasImage: boolean;
  imageSha256?: string;
  hasEmbedding: boolean;
  forRecognition: boolean;
  createdBy?: { id: number; username: string; name: string };
  createdAt: string;
}

export interface RenderedVersion {
  image: string;
  width: number;
  height: number;
  sha256: string;
  recipe: FaceRecipe;
  record: string;
}

function headers(json = true): Record<string, string> {
  const token = getToken();
  return { ...(json ? { "Content-Type": "application/json" } : {}), ...(token ? { Authorization: `Bearer ${token}` } : {}) };
}

const base = (caseId: string, evidenceId: number, traceId: number) =>
  `${apiBaseUrl}/cases/${encodeURIComponent(caseId)}/evidences/${evidenceId}/traces/${traceId}/codifications`;

async function failed(response: Response, fallback: string): Promise<never> {
  const body = (await response.json().catch(() => ({}))) as { error?: string };
  throw new Error(body.error ?? fallback);
}

export async function listCodificationVersions(caseId: string, evidenceId: number, traceId: number) {
  const response = await fetch(base(caseId, evidenceId, traceId), { headers: headers(false) });
  if (!response.ok) return failed(response, "Could not load the versions");
  return (await response.json()) as { versions: CodificationVersion[]; recipeVersion: number; software: string };
}

export async function previewCodificationVersion(
  caseId: string,
  evidenceId: number,
  traceId: number,
  recipe: FaceRecipe,
  signal?: AbortSignal,
): Promise<RenderedVersion> {
  const response = await fetch(`${base(caseId, evidenceId, traceId)}/preview`, {
    method: "POST",
    headers: headers(),
    body: JSON.stringify({ recipe }),
    signal,
  });
  if (!response.ok) return failed(response, "Could not render the version");
  return (await response.json()) as RenderedVersion;
}

export async function createCodificationVersion(
  caseId: string,
  evidenceId: number,
  traceId: number,
  recipe: FaceRecipe,
  label: string,
): Promise<CodificationVersion> {
  const response = await fetch(base(caseId, evidenceId, traceId), {
    method: "POST",
    headers: headers(),
    body: JSON.stringify({ recipe, label }),
  });
  if (!response.ok) return failed(response, "Could not save the version");
  return (await response.json()) as CodificationVersion;
}

export async function setRecognitionVersion(caseId: string, evidenceId: number, traceId: number, codificationId: number) {
  const response = await fetch(`${base(caseId, evidenceId, traceId)}/${codificationId}/recognition`, { method: "PUT", headers: headers(false) });
  if (!response.ok) return failed(response, "Could not choose the version");
}

export async function deleteCodificationVersion(caseId: string, evidenceId: number, traceId: number, codificationId: number) {
  const response = await fetch(`${base(caseId, evidenceId, traceId)}/${codificationId}`, { method: "DELETE", headers: headers(false) });
  if (!response.ok) return failed(response, "Could not delete the version");
}

/** A codification's own image as an object URL the caller revokes. */
export async function getCodificationVersionImageUrl(caseId: string, evidenceId: number, traceId: number, codificationId: number) {
  const response = await fetch(`${base(caseId, evidenceId, traceId)}/${codificationId}/image`, { headers: headers(false) });
  if (!response.ok) return failed(response, "Could not load the version's image");
  return URL.createObjectURL(await response.blob());
}
