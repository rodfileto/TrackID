import type { VideoLabFace } from "../../services/videolab";

/** A frame rate to step with until the real one is known. */
export const ASSUMED_FPS = 30;

/** One distinct colour per person, for boxes, timeline ticks and borders. */
const PERSON_COLORS = ["#2563eb", "#dc2626", "#16a34a", "#d97706", "#9333ea", "#0891b2", "#db2777", "#65a30d"];
export const personColor = (track: number) => PERSON_COLORS[track % PERSON_COLORS.length];

/** A person trackid-vision followed through a video: the best face and every face in time order. */
export interface VideoPerson {
  track: number;
  best: VideoLabFace;
  faces: VideoLabFace[];
  firstMs: number;
  lastMs: number;
}

/** The persons' faces at the sampled time nearest ms, within half an interval. */
export function personFacesAt(persons: VideoPerson[], ms: number, intervalMs: number) {
  const out: { track: number; face: VideoLabFace }[] = [];
  for (const p of persons) {
    for (const f of p.faces) {
      if (Math.abs(f.timestampMs - ms) <= intervalMs / 2) out.push({ track: p.track, face: f });
    }
  }
  return out;
}

export const sameFace = (a: VideoLabFace, b: VideoLabFace) =>
  a.timestampMs === b.timestampMs && a.box[0] === b.box[0] && a.box[1] === b.box[1];

/** Intersection over union of two boxes [x1, y1, x2, y2]. */
export function iou(a: readonly number[], b: readonly number[]) {
  const w = Math.min(a[2], b[2]) - Math.max(a[0], b[0]);
  const h = Math.min(a[3], b[3]) - Math.max(a[1], b[1]);
  if (w <= 0 || h <= 0) return 0;
  const inter = w * h;
  return inter / ((a[2] - a[0]) * (a[3] - a[1]) + (b[2] - b[0]) * (b[3] - b[1]) - inter);
}
