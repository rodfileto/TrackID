/**
 * TraceEditor
 *
 * A general-purpose manual trace-marking tool on top of ImageViewer: draw,
 * move, resize, and delete bounding boxes on an evidence image. It has no
 * idea whether it's marking a face or a fingerprint lift -- a trace is just
 * a box (see cases.TraceDetection on the backend, which is the same shape
 * for either case_type). Automatic detection (facial only) is a separate
 * concern that can populate `traces` the same way a person would.
 *
 * Controlled component: the caller owns the trace list via `traces` +
 * `onChange`.
 *
 * Usage:
 *   <TraceEditor src={objectUrl} traces={traces} onChange={setTraces} />
 */

import { useTranslation } from "react-i18next";
import { useEffect, useRef, useState } from "react";
import { Layer, Rect, Text, Transformer } from "react-konva";
import type Konva from "konva";
import { ImageViewer } from "../ui/image-viewer/ImageViewer";
import { PlusIcon, TrashBinIcon } from "../../icons";

// ─────────────────────────────────────────────────────────────────────────────
// Types
// ─────────────────────────────────────────────────────────────────────────────

/** One manually (or automatically) marked trace: a box in the original
 * image's pixel coordinates, mirroring the backend's TraceDetection. */
export interface TraceBox {
  /** Stable id for React keys/selection -- a client-generated uuid for a new
   * box, or the persisted case_traces id (as a string) for an existing one. */
  id: string;
  boxX1: number;
  boxY1: number;
  boxX2: number;
  boxY2: number;
  /** Detector confidence, when the box came from automatic detection. Not
   * set for hand-marked boxes. */
  score?: number;
  /** Marks a box as already persisted: renders read-only (can't be selected,
   * moved, resized, or deleted) regardless of `readOnly`. Use this for boxes
   * loaded from the server so editing only ever touches new, unsaved ones. */
  locked?: boolean;
}

export interface TraceEditorProps {
  src: string;
  traces: TraceBox[];
  onChange: (traces: TraceBox[]) => void;
  /** Disables adding/moving/resizing/deleting; boxes still render. */
  readOnly?: boolean;
  className?: string;
  /** Minimum box side, in original-image pixels, to keep a drawn box. */
  minBoxSize?: number;
  /** Controlled drawing mode, for callers that want their own "mark" button.
   * Leave undefined to let the editor's own + button manage it. */
  drawing?: boolean;
  onDrawingChange?: (drawing: boolean) => void;
}

const BOX_COLOR = "#3b82f6"; // blue-500
const SELECTED_COLOR = "#f59e0b"; // amber-500
const LOCKED_COLOR = "#9ca3af"; // gray-400
const DEFAULT_MIN_BOX_SIZE = 6;

function newTraceId(): string {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return crypto.randomUUID();
  }
  return `trace-${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

function isTypingTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  return (
    target.tagName === "INPUT" ||
    target.tagName === "TEXTAREA" ||
    target.isContentEditable
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// TraceEditor
// ─────────────────────────────────────────────────────────────────────────────

export default function TraceEditor({
  src,
  traces,
  onChange,
  readOnly = false,
  className = "",
  minBoxSize = DEFAULT_MIN_BOX_SIZE,
  drawing,
  onDrawingChange,
}: TraceEditorProps) {
  const { t } = useTranslation();
  const [internalDrawing, setInternalDrawing] = useState(false);
  const isDrawing = drawing ?? internalDrawing;
  function setIsDrawing(next: boolean) {
    setInternalDrawing(next);
    onDrawingChange?.(next);
  }
  const [draft, setDraft] = useState<{
    x1: number;
    y1: number;
    x2: number;
    y2: number;
  } | null>(null);
  const [selectedId, setSelectedId] = useState<string | null>(null);

  const rectRefs = useRef<Map<string, Konva.Rect>>(new Map());
  const transformerRef = useRef<Konva.Transformer>(null);
  const drawStart = useRef<{ x: number; y: number } | null>(null);

  // Attach the Transformer to the selected box's Rect node.
  useEffect(() => {
    const transformer = transformerRef.current;
    if (!transformer) return;
    const node = selectedId ? rectRefs.current.get(selectedId) : null;
    transformer.nodes(node ? [node] : []);
    transformer.getLayer()?.batchDraw();
  }, [selectedId, traces]);

  // Delete the selected box with Delete/Backspace (ignored while typing
  // elsewhere on the page, e.g. a description field).
  useEffect(() => {
    if (readOnly || !selectedId) return;
    function onKeyDown(e: KeyboardEvent) {
      if (isTypingTarget(e.target)) return;
      if (e.key === "Delete" || e.key === "Backspace") {
        e.preventDefault();
        removeSelected();
      }
    }
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [readOnly, selectedId, traces]);

  function removeSelected() {
    if (!selectedId) return;
    onChange(traces.filter((t) => t.id !== selectedId));
    setSelectedId(null);
  }

  function handleStageMouseDown(e: Konva.KonvaEventObject<MouseEvent>) {
    if (readOnly) return;
    const stage = e.target.getStage();
    if (!stage) return;

    if (isDrawing) {
      const point = stage.getRelativePointerPosition();
      if (!point) return;
      drawStart.current = point;
      setDraft({ x1: point.x, y1: point.y, x2: point.x, y2: point.y });
      return;
    }

    // A click that reaches the stage itself (not one of our boxes, which
    // stop propagation) means empty space -- deselect.
    if (e.target === stage) {
      setSelectedId(null);
    }
  }

  function handleStageMouseMove(e: Konva.KonvaEventObject<MouseEvent>) {
    if (!isDrawing || !drawStart.current) return;
    const stage = e.target.getStage();
    if (!stage) return;
    const point = stage.getRelativePointerPosition();
    if (!point) return;
    setDraft({
      x1: drawStart.current.x,
      y1: drawStart.current.y,
      x2: point.x,
      y2: point.y,
    });
  }

  function handleStageMouseUp() {
    if (!isDrawing) return;
    const current = draft;
    drawStart.current = null;
    setDraft(null);
    setIsDrawing(false);
    if (!current) return;

    const width = Math.abs(current.x2 - current.x1);
    const height = Math.abs(current.y2 - current.y1);
    if (width < minBoxSize || height < minBoxSize) return;

    const box: TraceBox = {
      id: newTraceId(),
      boxX1: Math.min(current.x1, current.x2),
      boxY1: Math.min(current.y1, current.y2),
      boxX2: Math.max(current.x1, current.x2),
      boxY2: Math.max(current.y1, current.y2),
    };
    onChange([...traces, box]);
    setSelectedId(box.id);
  }

  function updateBox(id: string, patch: Partial<TraceBox>) {
    onChange(traces.map((t) => (t.id === id ? { ...t, ...patch } : t)));
  }

  const draftBox = draft
    ? {
        x: Math.min(draft.x1, draft.x2),
        y: Math.min(draft.y1, draft.y2),
        width: Math.abs(draft.x2 - draft.x1),
        height: Math.abs(draft.y2 - draft.y1),
      }
    : null;

  return (
    <div className={`relative ${className}`}>
      <ImageViewer
        src={src}
        className="h-full w-full"
        draggable={!isDrawing}
        showRotate={false}
        onStageMouseDown={handleStageMouseDown}
        onStageMouseMove={handleStageMouseMove}
        onStageMouseUp={handleStageMouseUp}
        extraLayers={
          <Layer>
            {traces.map((trace) => {
              const selected = trace.id === selectedId;
              const editable = !readOnly && !trace.locked;
              const color = trace.locked
                ? LOCKED_COLOR
                : selected
                  ? SELECTED_COLOR
                  : BOX_COLOR;
              return (
                <Rect
                  key={trace.id}
                  ref={(node) => {
                    if (node) rectRefs.current.set(trace.id, node);
                    else rectRefs.current.delete(trace.id);
                  }}
                  x={trace.boxX1}
                  y={trace.boxY1}
                  width={trace.boxX2 - trace.boxX1}
                  height={trace.boxY2 - trace.boxY1}
                  stroke={color}
                  dash={trace.locked ? [4, 3] : undefined}
                  strokeWidth={2}
                  strokeScaleEnabled={false}
                  draggable={editable}
                  onMouseDown={(e) => {
                    e.cancelBubble = true;
                    if (editable) setSelectedId(trace.id);
                  }}
                  onDragEnd={(e) => {
                    const node = e.target;
                    updateBox(trace.id, {
                      boxX1: node.x(),
                      boxY1: node.y(),
                      boxX2: node.x() + node.width(),
                      boxY2: node.y() + node.height(),
                    });
                  }}
                  onTransformEnd={(e) => {
                    const node = e.target;
                    const scaleX = node.scaleX();
                    const scaleY = node.scaleY();
                    const width = Math.max(minBoxSize, node.width() * scaleX);
                    const height = Math.max(minBoxSize, node.height() * scaleY);
                    node.scaleX(1);
                    node.scaleY(1);
                    updateBox(trace.id, {
                      boxX1: node.x(),
                      boxY1: node.y(),
                      boxX2: node.x() + width,
                      boxY2: node.y() + height,
                    });
                  }}
                />
              );
            })}
            {traces.map((trace, index) => (
              <Text
                key={`${trace.id}-label`}
                x={trace.boxX1}
                y={trace.boxY1 - 16}
                text={String(index + 1)}
                fontSize={13}
                fill={
                  trace.locked
                    ? LOCKED_COLOR
                    : trace.id === selectedId
                      ? SELECTED_COLOR
                      : BOX_COLOR
                }
                listening={false}
              />
            ))}
            {draftBox && (
              <Rect
                x={draftBox.x}
                y={draftBox.y}
                width={draftBox.width}
                height={draftBox.height}
                stroke={SELECTED_COLOR}
                dash={[6, 4]}
                strokeWidth={2}
                strokeScaleEnabled={false}
                listening={false}
              />
            )}
            {!readOnly && (
              <Transformer
                ref={transformerRef}
                rotateEnabled={false}
                flipEnabled={false}
                borderStroke={SELECTED_COLOR}
                anchorStroke={SELECTED_COLOR}
                anchorFill="#fff"
                anchorSize={8}
                keepRatio={false}
              />
            )}
          </Layer>
        }
      />

      {!readOnly && (
        <div className="absolute top-2 right-2 z-10 flex items-center gap-0.5 rounded-lg bg-black/50 px-1.5 py-1 backdrop-blur-sm">
          <button
            type="button"
            onClick={() => {
              setSelectedId(null);
              setIsDrawing(!isDrawing);
            }}
            title={t("traceEditor.mark")}
            className={`flex h-6 w-6 items-center justify-center rounded transition-colors ${
              isDrawing
                ? "bg-brand-500 text-white"
                : "text-white hover:bg-white/10 hover:text-blue-300"
            }`}
          >
            <PlusIcon className="h-4 w-4" />
          </button>
          <button
            type="button"
            onClick={removeSelected}
            disabled={!selectedId}
            title={t("traceEditor.deleteSelected")}
            className="flex h-6 w-6 items-center justify-center rounded text-white transition-colors hover:bg-white/10 hover:text-error-400 disabled:opacity-30 disabled:hover:bg-transparent disabled:hover:text-white"
          >
            <TrashBinIcon className="h-4 w-4" />
          </button>
        </div>
      )}
    </div>
  );
}
