import { apiBaseUrl, requestApi } from "./api";
import { getToken } from "./auth";
import { SourceExpiredError, type Interpolation, type Rect } from "./imageproc";
import type { Conclusion } from "./analyses";

// The face lab's API (api/facelab_handler.go): a one-to-one facial comparison after FISWG on any
// two images, nothing stored. The types mirror facecompare and imageproc.FaceRecipe.

export { SourceExpiredError };
export type { Rect, Interpolation };

export type Affine = { A: number; B: number; C: number; D: number; E: number; F: number };
export type Point = [number, number];
export type ImageCode = "Q" | "K";

export interface FaceRecipe {
  version?: number;
  crop?: Rect;
  rotate: number;
  mirror: boolean;
  scale: number;
  interpolation: Interpolation;
  colorBalance?: { red: number; green: number; blue: number };
  grey?: "" | "luminance" | "red" | "green" | "blue";
  brightnessContrast?: { brightness: number; contrast: number };
  levels?: { inBlack: number; inWhite: number; gamma: number; outBlack: number; outWhite: number };
  curves?: { points: Point[] };
}

export const IDENTITY_RECIPE: FaceRecipe = { rotate: 0, mirror: false, scale: 1, interpolation: "bicubic" };

export interface Label {
  code: string;
  en: string;
  pt: string;
}

export type Stability = "H" | "M" | "L";
export type StabilityFactor = "EXPRESSION" | "SHORT_TERM" | "LONG_TERM" | "WEIGHT" | "HEALTH" | "ALTERATION";

export interface Characteristic {
  code: string;
  en: string;
  pt: string;
  sided?: boolean;
  descriptors: string[];
  stability?: Record<StabilityFactor, Stability> & { page: number };
}

export interface Component {
  code: string;
  en: string;
  pt: string;
  characteristics: Characteristic[];
}

export interface ScaleLevel {
  code: Conclusion;
  en: string;
  pt: string;
  meaningPt: string;
}

export interface Reference {
  code: string;
  title: string;
  version: string;
  sections: string;
  use: string;
}

export interface FaceLabInfo {
  components: Component[];
  descriptors: Record<string, Label>;
  stabilityFactors: Label[];
  imageFactors: Label[];
  effects: Label[];
  suitabilities: Label[];
  results: Label[];
  explanations: Label[];
  markupKinds: Label[];
  scale: ScaleLevel[];
  references: Reference[];
  featureListSource: string;
  stabilitySource: string;
  recipeVersion: number;
  sessionVersion: number;
  lowIpd: number;
  detection: boolean;
  software: string;
}

export interface DetectedFace {
  box: Rect;
  score: number;
  landmarks: [Point, Point, Point, Point, Point];
}

export interface Exif {
  dateTimeOriginal?: string;
  dateTime?: string;
  make?: string;
  model?: string;
  software?: string;
}

export interface FaceSource {
  id: string;
  filename: string;
  format: string;
  width: number;
  height: number;
  sha256: string;
  orientation: number;
  exif: Exif;
  faces: DetectedFace[];
}

export interface FaceRender {
  image: string;
  width: number;
  height: number;
  sha256: string;
  histogram: number[];
  transform: Affine;
  recipe: FaceRecipe;
  record: string;
}

export interface Landmarks {
  points: [Point, Point, Point, Point, Point];
  source: "DETECTOR" | "EXAMINER";
}

export interface ImageFactor {
  code: string;
  effect: "NONE" | "MINOR" | "SIGNIFICANT";
  notes?: string;
}

export interface Analysis {
  factors: ImageFactor[];
  mirrored: boolean;
  manipulationSuspected: boolean;
  manipulationNotes?: string;
  captureDate?: string;
  suitability?: "" | "SUITABLE" | "LEAD_ONLY" | "UNSUITABLE";
  suitabilityReason?: string;
  notes?: string;
  done: boolean;
}

export interface Side {
  sourceId: string;
  recipe: FaceRecipe;
  face?: Rect;
  landmarks?: Landmarks;
  analysis: Analysis;
}

export interface PointPair {
  q: Point;
  k: Point;
}

export type MarkupKind = "ELLIPSE" | "BOX" | "BRACKET" | "ARROW" | "POINT";

export interface Markup {
  image: ImageCode;
  number: number;
  kind: MarkupKind;
  points: Point[];
}

export type FindingResult = "SIMILAR" | "DISSIMILAR" | "NOT_COMPARABLE";

export interface Finding {
  characteristic: string;
  side?: "" | "RIGHT" | "LEFT";
  descriptors?: string[];
  questioned?: string;
  known?: string;
  result: FindingResult;
  /** One or more causes of a DISSIMILAR result, or UNEXPLAINED alone. */
  explanations?: string[];
  /** The single explanation of sessions saved before 2026-10-30; read on load only. */
  explanation?: string;
  notes?: string;
  markups?: number[];
}

export interface Opinion {
  conclusion: Conclusion;
  reasoning: string;
}

export interface Session {
  version: number;
  questioned: Side;
  known: Side;
  questionedFirst: boolean;
  pairs?: PointPair[];
  superimposition: boolean;
  factors?: StabilityFactor[];
  markups?: Markup[];
  findings?: Finding[];
  opinion?: Opinion;
}

export interface Pose {
  ipd: number;
  roll: number;
  yaw: number;
  pitchIndex: number;
}

export interface Metrics {
  pose?: Pose;
  width: number;
  height: number;
  sharpness: number;
  clippedDark: number;
  clippedBright: number;
  meanLuminance: number;
}

export interface Similarity {
  transform: Affine;
  scale: number;
  rotation: number;
  rms: number;
  pairs: number;
}

export interface Warning {
  code: string;
  message: string;
  where?: string;
}

export interface Summary {
  similar: number;
  dissimilar: number;
  explained: number;
  notComparable: number;
  similarByStability: Record<string, number>;
  dissimilarByStability: Record<string, number>;
  componentsCompared: number;
}

export interface SheetResult {
  annotated: string;
  annotatedSha256: string;
  clean: string;
  cleanSha256: string;
  record: string;
  warnings: Warning[];
  summary: Summary;
}

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

export function getFaceLabInfo(): Promise<FaceLabInfo> {
  return requestApi<FaceLabInfo>("/facelab", { headers: authHeaders() });
}

export async function uploadFaceSource(file: File): Promise<FaceSource> {
  const form = new FormData();
  form.append("file", file);
  const response = await fetch(`${apiBaseUrl}/facelab/sources`, { method: "POST", headers: authHeaders(), body: form });
  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(body.error ?? `Could not read ${file.name}`);
  }
  return (await response.json()) as FaceSource;
}

/** The source as the server still holds it, or SourceExpiredError. */
export async function getFaceSource(id: string): Promise<FaceSource> {
  const response = await fetch(`${apiBaseUrl}/facelab/sources/${id}`, { headers: authHeaders() });
  if (response.status === 410) throw new SourceExpiredError("expired");
  if (!response.ok) throw new Error("The request could not be completed");
  return (await response.json()) as FaceSource;
}

export function renderFace(sourceId: string, recipe: FaceRecipe, signal?: AbortSignal) {
  return post<FaceRender>("/facelab/render", { sourceId, recipe }, signal);
}

export function measureFace(sourceId: string, face?: Rect, landmarks?: Landmarks) {
  return post<Metrics>("/facelab/quality", { sourceId, face, landmarks });
}

export function alignFaces(questioned: Side, known: Side, pairs: PointPair[]) {
  return post<{ alignment: Similarity; pairs: PointPair[]; warnings: Warning[] }>("/facelab/align", { questioned, known, pairs });
}

export function checkSession(session: Session, signal?: AbortSignal) {
  return post<{ warnings: Warning[]; summary: Summary }>("/facelab/check", { session }, signal);
}

export function sheetFor(session: Session) {
  return post<SheetResult>("/facelab/sheet", { session });
}
