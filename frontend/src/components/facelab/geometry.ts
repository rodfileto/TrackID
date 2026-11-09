import type { Affine, Point } from "../../services/facelab";

// Affine maps as imageproc.Affine writes them: x' = A x + B y + C, y' = D x + E y + F.

export const IDENTITY: Affine = { A: 1, B: 0, C: 0, D: 0, E: 1, F: 0 };

export function apply(m: Affine, p: Point): Point {
  return [m.A * p[0] + m.B * p[1] + m.C, m.D * p[0] + m.E * p[1] + m.F];
}

export function invert(m: Affine): Affine {
  const det = m.A * m.E - m.B * m.D;
  const a = m.E / det, b = -m.B / det, d = -m.D / det, e = m.A / det;
  return { A: a, B: b, C: -(a * m.C + b * m.F), D: d, E: e, F: -(d * m.C + e * m.F) };
}

/** first, then second. */
export function compose(first: Affine, second: Affine): Affine {
  return {
    A: second.A * first.A + second.B * first.D,
    B: second.A * first.B + second.B * first.E,
    C: second.A * first.C + second.B * first.F + second.C,
    D: second.D * first.A + second.E * first.D,
    E: second.D * first.B + second.E * first.E,
    F: second.D * first.C + second.E * first.F + second.F,
  };
}

/** A view: rendered pixels to screen, by a uniform zoom and an offset. */
export interface View {
  scale: number;
  x: number;
  y: number;
}

export function viewAffine(v: View): Affine {
  return { A: v.scale, B: 0, C: v.x, D: 0, E: v.scale, F: v.y };
}

/** Konva's position, rotation and scale for a map with no shear (a similarity, or a mirror of
 * one). Konva applies scale, then rotation, then translation. */
export function konvaAttrs(m: Affine) {
  const scaleX = Math.hypot(m.A, m.D);
  const det = m.A * m.E - m.B * m.D;
  const scaleY = det / scaleX;
  return { x: m.C, y: m.F, rotation: (Math.atan2(m.D, m.A) * 180) / Math.PI, scaleX, scaleY };
}

export function dist(a: Point, b: Point) {
  return Math.hypot(a[0] - b[0], a[1] - b[1]);
}
