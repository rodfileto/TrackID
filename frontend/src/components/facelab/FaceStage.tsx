import { useEffect, useMemo, useRef, useState } from "react";
import { Arrow, Circle, Group, Image as KonvaImage, Layer, Line, Rect as KonvaRect, Text } from "react-konva";
import type Konva from "konva";

import { ImageViewer } from "../ui/image-viewer/ImageViewer";
import type { Affine, ImageCode, Landmarks, Markup, MarkupKind, Point, PointPair, Rect } from "../../services/facelab";
import { apply, compose, dist, invert, konvaAttrs, viewAffine, type View } from "./geometry";

// One viewer showing one face (side by side) or both (overlay, wipe, flicker), built on
// ImageViewer: its stage, zoom, pan and fit, with a controlled view so linked panels share it. The
// first image is the viewer's own; the others, and everything the examiner places, are extra
// layers. What the examiner places is kept in each image's original pixels and drawn through its
// processing (toRender) and, for the known image once aligned, the alignment into the questioned
// image's frame (toFrame). The view maps the frame to the screen.

export type Tool = "pan" | "select" | "crop" | "landmarks" | "pair" | "guideH" | "guideV" | MarkupKind;

export interface StageImage {
  code: ImageCode;
  src: string;
  width: number;
  height: number;
  toRender: Affine;
  toFrame: Affine;
  opacity?: number;
  visible?: boolean;
}

export interface Guide {
  axis: "h" | "v";
  at: number; // in frame pixels
}

export const MARKUP_COLOUR = "#ffd600";
const LANDMARK_COLOUR = "#22d3ee";
const PAIR_COLOUR = "#e879f9";
const GUIDE_COLOUR = "#38bdf8";
const CROP_COLOUR = "#f59e0b";

interface FaceStageProps {
  images: StageImage[]; // bottom to top; the first is the viewer's own image
  /** null until fitted: the viewer then fits its image and reports the view. */
  view: View | null;
  onView: (v: View) => void;
  tool: Tool;
  /** The image drawing goes to, when the stage shows two. */
  target: ImageCode;
  markups: Markup[];
  selected: number | null;
  onSelect: (index: number | null) => void;
  onAddMarkup: (m: Omit<Markup, "number">) => void;
  onMoveMarkup: (index: number, points: Point[]) => void;
  landmarks: Partial<Record<ImageCode, Landmarks>>;
  showLandmarks: boolean;
  onLandmark: (code: ImageCode, index: number, p: Point) => void;
  pairs: PointPair[];
  pendingPair: Point | null; // a questioned point waiting for its known one
  showPairs: boolean;
  onPairPoint: (code: ImageCode, p: Point) => void;
  guides: Guide[];
  onGuides: (g: Guide[]) => void;
  onCrop: (code: ImageCode, r: Rect) => void;
  /** Where the wipe splits the top image off, as an x in frame pixels, or null. */
  wipe: number | null;
  onWipe: (x: number) => void;
}

function useLoadedImage(src: string) {
  const [img, setImg] = useState<HTMLImageElement | null>(null);
  useEffect(() => {
    const el = new window.Image();
    el.onload = () => setImg(el);
    el.src = src;
    return () => {
      el.onload = null;
    };
  }, [src]);
  return img;
}

function ImageNode({ image }: { image: StageImage }) {
  const img = useLoadedImage(image.src);
  return img ? <KonvaImage image={img} width={image.width} height={image.height} /> : null;
}

/** The points of m drawn in rendered pixels; ellipses and boxes become polygons, since the
 * processing may rotate them. */
function markupShape(m: Markup, t: Affine) {
  const r = (p: Point) => apply(t, p);
  if (m.kind === "BOX") {
    const [[x1, y1], [x2, y2]] = m.points;
    return [r([x1, y1]), r([x2, y1]), r([x2, y2]), r([x1, y2])];
  }
  if (m.kind === "ELLIPSE") {
    const [[x1, y1], [x2, y2]] = m.points;
    const cx = (x1 + x2) / 2, cy = (y1 + y2) / 2, rx = Math.abs(x2 - x1) / 2, ry = Math.abs(y2 - y1) / 2;
    return Array.from({ length: 72 }, (_, i) => {
      const a = (i / 72) * 2 * Math.PI;
      return r([cx + rx * Math.cos(a), cy + ry * Math.sin(a)]);
    });
  }
  return m.points.map(r);
}

function MarkupNode({ m, t, px, selected, draggable, onSelect, onMove }: {
  m: Markup; t: Affine; px: number; selected: boolean; draggable: boolean;
  onSelect: () => void; onMove: (delta: Point) => void;
}) {
  const pts = markupShape(m, t);
  const stroke = selected ? "#ef4444" : MARKUP_COLOUR;
  const common = { stroke, strokeWidth: 2.5, strokeScaleEnabled: false, shadowColor: "black", shadowBlur: 2, shadowOpacity: 0.9, hitStrokeWidth: 12 };
  let shape: React.ReactNode;
  let anchor: Point;
  if (m.kind === "BOX" || m.kind === "ELLIPSE") {
    shape = <Line points={pts.flat()} closed {...common} />;
    anchor = [Math.min(...pts.map((p) => p[0])) - 14 * px, Math.min(...pts.map((p) => p[1])) - 16 * px];
  } else if (m.kind === "BRACKET") {
    const [a, b] = pts;
    const l = dist(a, b) || 1;
    const n: Point = [-(b[1] - a[1]) / l, (b[0] - a[0]) / l];
    const tick = 8 * px;
    shape = (
      <>
        <Line points={[...a, ...b]} {...common} />
        <Line points={[a[0] - n[0] * tick, a[1] - n[1] * tick, a[0] + n[0] * tick, a[1] + n[1] * tick]} {...common} />
        <Line points={[b[0] - n[0] * tick, b[1] - n[1] * tick, b[0] + n[0] * tick, b[1] + n[1] * tick]} {...common} />
      </>
    );
    anchor = [(a[0] + b[0]) / 2 + n[0] * 16 * px - 5 * px, (a[1] + b[1]) / 2 + n[1] * 16 * px - 8 * px];
  } else if (m.kind === "ARROW") {
    const [a, b] = pts;
    shape = <Arrow points={[...a, ...b]} pointerLength={10 * px} pointerWidth={9 * px} fill={stroke} {...common} />;
    const l = dist(a, b) || 1;
    anchor = [a[0] - ((b[0] - a[0]) / l) * 14 * px - 5 * px, a[1] - ((b[1] - a[1]) / l) * 14 * px - 8 * px];
  } else {
    const [p] = pts;
    shape = <Circle x={p[0]} y={p[1]} radius={4.5 * px} fill={stroke} stroke="black" strokeWidth={1} strokeScaleEnabled={false} />;
    anchor = [p[0] + 7 * px, p[1] - 18 * px];
  }
  return (
    <Group
      draggable={draggable}
      onMouseDown={(e) => {
        if (!draggable) return;
        e.cancelBubble = true;
        onSelect();
      }}
      onDragEnd={(e) => {
        const g = e.target;
        const delta: Point = [g.x(), g.y()];
        g.position({ x: 0, y: 0 });
        if (delta[0] || delta[1]) onMove(delta);
      }}
    >
      {shape}
      <Text x={anchor[0]} y={anchor[1]} text={String(m.number)} fontSize={18 * px} fontStyle="bold" fill={stroke}
        stroke="black" strokeWidth={0.6 * px} />
    </Group>
  );
}

const LANDMARK_NAMES = ["1", "2", "N", "B", "B"];

const BIG = 1e6; // frame pixels: lines and clips that reach past any image

export default function FaceStage(props: FaceStageProps) {
  const { images, view, onView, tool, target, markups, selected, landmarks, pairs, guides } = props;
  const drag = useRef<{ original: Point; code: ImageCode } | null>(null);
  const [draft, setDraft] = useState<{ code: ImageCode; a: Point; b: Point } | null>(null);
  const [base, ...others] = images;

  // Original pixels to screen, per image.
  const toScreen = useMemo(() => {
    const out: Partial<Record<ImageCode, Affine>> = {};
    if (view) for (const im of images) out[im.code] = compose(compose(im.toRender, im.toFrame), viewAffine(view));
    return out;
  }, [images, view]);

  const targetImage = images.find((i) => i.code === target) ?? images[images.length - 1];
  const panning = tool === "pan" || tool === "select" || tool === "landmarks";

  function pointer(e: Konva.KonvaEventObject<MouseEvent>): Point | null {
    const p = e.target.getStage()?.getPointerPosition();
    return p ? [p.x, p.y] : null;
  }

  function toOriginal(code: ImageCode, screen: Point): Point {
    return apply(invert(toScreen[code]!), screen);
  }

  function down(e: Konva.KonvaEventObject<MouseEvent>) {
    const p = pointer(e);
    if (!p || !targetImage || !view) return;
    if (tool === "select" && e.target === e.target.getStage()) props.onSelect(null);
    if (panning) return; // the viewer pans
    const code = targetImage.code;
    const o = toOriginal(code, p);
    if (tool === "POINT") {
      props.onAddMarkup({ image: code, kind: "POINT", points: [o] });
      return;
    }
    if (tool === "pair") {
      props.onPairPoint(code, o);
      return;
    }
    if (tool === "guideH" || tool === "guideV") {
      const frame = apply(invert(viewAffine(view)), p);
      props.onGuides([...guides, { axis: tool === "guideH" ? "h" : "v", at: tool === "guideH" ? frame[1] : frame[0] }]);
      return;
    }
    drag.current = { original: o, code };
    setDraft({ code, a: o, b: o });
  }

  function move(e: Konva.KonvaEventObject<MouseEvent>) {
    const d = drag.current;
    const p = pointer(e);
    if (d && p && view) setDraft({ code: d.code, a: d.original, b: toOriginal(d.code, p) });
  }

  function up() {
    const d = drag.current;
    drag.current = null;
    const dr = draft;
    setDraft(null);
    if (!d || !dr || !view) return;
    const screenLength = dist(apply(toScreen[dr.code]!, dr.a), apply(toScreen[dr.code]!, dr.b));
    if (screenLength < 6) return;
    if (tool === "crop") {
      // A box drawn on the screen is a turned box on the original when the image is rotated: crop
      // to its bounds there.
      const [a, b] = [dr.a, dr.b].map((q) => apply(toScreen[dr.code]!, q));
      const orig = ([a, b, [a[0], b[1]], [b[0], a[1]]] as Point[]).map((q) => toOriginal(dr.code, q));
      props.onCrop(dr.code, {
        x1: Math.round(Math.min(...orig.map((q) => q[0]))), y1: Math.round(Math.min(...orig.map((q) => q[1]))),
        x2: Math.round(Math.max(...orig.map((q) => q[0]))), y2: Math.round(Math.max(...orig.map((q) => q[1]))),
      });
      return;
    }
    if (tool === "ELLIPSE" || tool === "BOX" || tool === "BRACKET" || tool === "ARROW") {
      props.onAddMarkup({ image: dr.code, kind: tool, points: [dr.a, dr.b] });
    }
  }

  /** What the examiner placed on im, in its rendered pixels; px is one screen pixel there. */
  function overlays(im: StageImage, px: number) {
    const lm = landmarks[im.code];
    return (
      <>
        {markups.map((m, i) =>
          m.image === im.code ? (
            <MarkupNode key={i} m={m} t={im.toRender} px={px} selected={selected === i} draggable={tool === "select"}
              onSelect={() => props.onSelect(i)}
              onMove={(delta) => {
                const back = invert(im.toRender);
                props.onMoveMarkup(i, m.points.map((p) => {
                  const r = apply(im.toRender, p);
                  return apply(back, [r[0] + delta[0], r[1] + delta[1]]);
                }));
              }} />
          ) : null,
        )}
        {props.showLandmarks && lm && lm.points.map((p, i) => {
          const r = apply(im.toRender, p);
          return (
            <Group key={`lm${i}`} x={r[0]} y={r[1]} draggable={tool === "landmarks"}
              onMouseDown={(e) => { if (tool === "landmarks") e.cancelBubble = true; }}
              onDragEnd={(e) => {
                const g = e.target;
                props.onLandmark(im.code, i, apply(invert(im.toRender), [g.x(), g.y()]));
              }}>
              <Circle radius={5 * px} stroke={LANDMARK_COLOUR} strokeWidth={2} strokeScaleEnabled={false} />
              <Line points={[-8 * px, 0, 8 * px, 0]} stroke={LANDMARK_COLOUR} strokeWidth={1} strokeScaleEnabled={false} />
              <Line points={[0, -8 * px, 0, 8 * px]} stroke={LANDMARK_COLOUR} strokeWidth={1} strokeScaleEnabled={false} />
              <Text x={7 * px} y={-16 * px} text={LANDMARK_NAMES[i]} fontSize={13 * px} fill={LANDMARK_COLOUR} />
            </Group>
          );
        })}
        {props.showPairs && pairs.map((pair, i) => {
          const r = apply(im.toRender, im.code === "Q" ? pair.q : pair.k);
          return (
            <Group key={`pair${i}`} x={r[0]} y={r[1]}>
              <Circle radius={4 * px} fill={PAIR_COLOUR} stroke="black" strokeWidth={1} strokeScaleEnabled={false} />
              <Text x={6 * px} y={4 * px} text={`P${i + 1}`} fontSize={13 * px} fill={PAIR_COLOUR} />
            </Group>
          );
        })}
        {props.pendingPair && im.code === "Q" && (() => {
          const r = apply(im.toRender, props.pendingPair);
          return <Circle x={r[0]} y={r[1]} radius={6 * px} stroke={PAIR_COLOUR} strokeWidth={2} dash={[3, 3]} strokeScaleEnabled={false} />;
        })()}
        {draft && draft.code === im.code && (() => {
          if (tool === "crop") {
            const a = apply(im.toRender, draft.a), b = apply(im.toRender, draft.b);
            return <KonvaRect x={Math.min(a[0], b[0])} y={Math.min(a[1], b[1])} width={Math.abs(b[0] - a[0])} height={Math.abs(b[1] - a[1])}
              stroke={CROP_COLOUR} strokeWidth={2} dash={[6, 4]} strokeScaleEnabled={false} />;
          }
          if (tool !== "ELLIPSE" && tool !== "BOX" && tool !== "BRACKET" && tool !== "ARROW") return null;
          const m: Markup = { image: im.code, number: 0, kind: tool, points: [draft.a, draft.b] };
          return <MarkupNode m={m} t={im.toRender} px={px} selected={false} draggable={false} onSelect={() => {}} onMove={() => {}} />;
        })()}
      </>
    );
  }

  // The viewer's stage carries the view, so layers are in frame pixels.
  const zoom = view?.scale ?? 1;
  const pxFor = (im: StageImage) => 1 / (zoom * Math.abs(konvaAttrs(im.toFrame).scaleX));
  const wipe = props.wipe;

  const extraLayers = (
    <>
      {others.map((im) => (
        <Layer key={im.code} visible={im.visible !== false} opacity={im.opacity ?? 1}
          {...(wipe != null ? { clipX: wipe, clipY: -BIG, clipWidth: 2 * BIG, clipHeight: 2 * BIG } : {})}>
          <Group {...konvaAttrs(im.toFrame)}>
            <ImageNode image={im} />
            {overlays(im, pxFor(im))}
          </Group>
        </Layer>
      ))}
      <Layer>
        <Group {...konvaAttrs(base.toFrame)}>{overlays(base, pxFor(base))}</Group>
        {guides.map((g, i) => (
          <Line key={i} points={g.axis === "h" ? [-BIG, g.at, BIG, g.at] : [g.at, -BIG, g.at, BIG]}
            stroke={GUIDE_COLOUR} strokeWidth={1} dash={[8, 5]} strokeScaleEnabled={false} hitStrokeWidth={10}
            draggable={tool === "select" || tool === "guideH" || tool === "guideV"}
            dragBoundFunc={function (this: Konva.Node, pos) {
              return g.axis === "h" ? { x: this.absolutePosition().x, y: pos.y } : { x: pos.x, y: this.absolutePosition().y };
            }}
            onMouseDown={(e) => { e.cancelBubble = true; }}
            onDblClick={() => props.onGuides(guides.filter((_, j) => j !== i))}
            onDragEnd={(e) => {
              const delta = g.axis === "h" ? e.target.y() : e.target.x();
              e.target.position({ x: 0, y: 0 });
              props.onGuides(guides.map((x, j) => (j === i ? { ...x, at: x.at + delta } : x)));
            }} />
        ))}
        {wipe != null && (
          <Line points={[wipe, -BIG, wipe, BIG]} stroke="white" strokeWidth={3} strokeScaleEnabled={false} hitStrokeWidth={16} draggable
            dragBoundFunc={function (this: Konva.Node, pos) { return { x: pos.x, y: this.absolutePosition().y }; }}
            onMouseDown={(e) => { e.cancelBubble = true; }}
            onDragMove={(e) => {
              const x = wipe + e.target.x();
              e.target.position({ x: 0, y: 0 });
              props.onWipe(x);
            }} />
        )}
      </Layer>
    </>
  );

  return (
    <ImageViewer
      src={base.src}
      className="h-[560px] w-full rounded-lg"
      view={view}
      onViewChange={onView}
      imageTransform={konvaAttrs(base.toFrame)}
      keepView
      showRotate={false}
      draggable={panning}
      cursor={panning ? undefined : "crosshair"}
      onStageMouseDown={down}
      onStageMouseMove={move}
      onStageMouseUp={up}
      extraLayers={extraLayers}
    />
  );
}
