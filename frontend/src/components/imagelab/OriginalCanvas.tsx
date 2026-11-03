import { useRef, useState } from "react";
import { Circle, Layer, Line, Rect as KonvaRect, Text } from "react-konva";
import type Konva from "konva";
import { ImageViewer } from "../ui/image-viewer/ImageViewer";
import type { Rect } from "../../services/imageproc";

export type DrawMode = "none" | "crop" | "line";

interface OriginalCanvasProps {
  src: string;
  crop?: Rect;
  line?: Rect;
  mode: DrawMode;
  onCrop: (crop: Rect) => void;
  onLine: (line: Rect) => void;
  /** Called when a drag ends, so the caller can leave the drawing mode. */
  onDone: () => void;
  className?: string;
}

const CROP_COLOR = "#f59e0b";
const LINE_COLOR = "#ef4444";

/** The original image with the lab's two drawn marks, both in the original's pixels: the crop
 * (a box) and the calibration line drawn along a scale. */
export default function OriginalCanvas({ src, crop, line, mode, onCrop, onLine, onDone, className = "" }: OriginalCanvasProps) {
  const start = useRef<{ x: number; y: number } | null>(null);
  const [draft, setDraft] = useState<Rect | null>(null);

  function point(e: Konva.KonvaEventObject<MouseEvent>) {
    return e.target.getStage()?.getRelativePointerPosition() ?? null;
  }

  function down(e: Konva.KonvaEventObject<MouseEvent>) {
    if (mode === "none") return;
    const p = point(e);
    if (!p) return;
    start.current = p;
    setDraft({ x1: p.x, y1: p.y, x2: p.x, y2: p.y });
  }

  function move(e: Konva.KonvaEventObject<MouseEvent>) {
    if (!start.current) return;
    const p = point(e);
    if (p) setDraft({ x1: start.current.x, y1: start.current.y, x2: p.x, y2: p.y });
  }

  function up() {
    const d = draft;
    start.current = null;
    setDraft(null);
    if (!d) return;
    if (mode === "crop" && Math.abs(d.x2 - d.x1) >= 8 && Math.abs(d.y2 - d.y1) >= 8) {
      onCrop({
        x1: Math.round(Math.min(d.x1, d.x2)),
        y1: Math.round(Math.min(d.y1, d.y2)),
        x2: Math.round(Math.max(d.x1, d.x2)),
        y2: Math.round(Math.max(d.y1, d.y2)),
      });
    }
    if (mode === "line" && Math.hypot(d.x2 - d.x1, d.y2 - d.y1) >= 10) {
      onLine({ x1: d.x1, y1: d.y1, x2: d.x2, y2: d.y2 });
    }
    onDone();
  }

  const shownCrop = mode === "crop" && draft ? draft : crop;
  const shownLine = mode === "line" && draft ? draft : line;

  return (
    <ImageViewer
      src={src}
      className={className}
      draggable={mode === "none"}
      showRotate={false}
      onStageMouseDown={down}
      onStageMouseMove={move}
      onStageMouseUp={up}
      extraLayers={
        <Layer listening={false}>
          {shownCrop && (
            <KonvaRect
              x={Math.min(shownCrop.x1, shownCrop.x2)}
              y={Math.min(shownCrop.y1, shownCrop.y2)}
              width={Math.abs(shownCrop.x2 - shownCrop.x1)}
              height={Math.abs(shownCrop.y2 - shownCrop.y1)}
              stroke={CROP_COLOR}
              strokeWidth={2}
              dash={[8, 4]}
              strokeScaleEnabled={false}
            />
          )}
          {shownLine && (
            <>
              <Line
                points={[shownLine.x1, shownLine.y1, shownLine.x2, shownLine.y2]}
                stroke={LINE_COLOR}
                strokeWidth={2}
                strokeScaleEnabled={false}
              />
              <Circle x={shownLine.x1} y={shownLine.y1} radius={4} fill={LINE_COLOR} strokeScaleEnabled={false} />
              <Circle x={shownLine.x2} y={shownLine.y2} radius={4} fill={LINE_COLOR} strokeScaleEnabled={false} />
              <Text
                x={(shownLine.x1 + shownLine.x2) / 2 + 6}
                y={(shownLine.y1 + shownLine.y2) / 2 + 6}
                text={`${Math.hypot(shownLine.x2 - shownLine.x1, shownLine.y2 - shownLine.y1).toFixed(1)} px`}
                fill={LINE_COLOR}
                fontSize={14}
                fontStyle="bold"
              />
            </>
          )}
        </Layer>
      }
    />
  );
}
