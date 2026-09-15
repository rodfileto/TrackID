/**
 * ImageViewer
 *
 * A reusable Konva-based image viewer with:
 *  - Mouse-wheel zoom (cursor-centered)
 *  - Drag to pan
 *  - Fit-to-container button (auto-fires on load)
 *  - Rotate 90° CW button
 *  - Zoom % indicator
 *  - Optional brightness/contrast/saturation + bicubic/bilinear/nearest-
 *    neighbor interpolation toggle (`showFilters`)
 *  - `extraLayers` prop for annotation layers (see TraceEditor)
 *
 * Usage:
 *   <ImageViewer src={objectUrl} className="w-full h-64" />
 */

import React, {
  forwardRef,
  useState,
  useRef,
  useEffect,
  useCallback,
  useImperativeHandle,
} from "react";
import { Stage, Layer, Image as KonvaImage } from "react-konva";
import Konva from "konva";
import {
  ZoomInIcon,
  ZoomOutIcon,
  FitToFrameIcon,
  RotateIcon,
} from "./viewer-icons";

// ─────────────────────────────────────────────────────────────────────────────
// Constants
// ─────────────────────────────────────────────────────────────────────────────

const DEFAULT_MIN_SCALE = 0.05;
const DEFAULT_MAX_SCALE = 20;
const ZOOM_FACTOR = 1.18;

// Konva.Filters.Brighten/Contrast/HSV value ranges.
const BRIGHTNESS_RANGE = { min: -1, max: 1, step: 0.05 };
const CONTRAST_RANGE = { min: -100, max: 100, step: 5 };
const SATURATION_RANGE = { min: -2, max: 2, step: 0.1 };

// Canvas imageSmoothingEnabled/imageSmoothingQuality don't let you pick an
// exact resampling algorithm -- browsers choose one for each quality level --
// but "high" is bicubic-like (smoothest, best for photos) and "low" is
// bilinear-like (cheaper, still smooth) in every mainstream engine. "nearest"
// disables smoothing entirely for a crisp, blocky pixel-for-pixel view.
type Interpolation = "bicubic" | "bilinear" | "nearest";

const INTERPOLATION_OPTIONS: Array<{
  value: Interpolation;
  label: string;
  title: string;
}> = [
  {
    value: "bicubic",
    label: "Bicubic",
    title: "Smoothest -- best for standard photographs",
  },
  {
    value: "bilinear",
    label: "Bilinear",
    title: "Smooth, cheaper to compute",
  },
  {
    value: "nearest",
    label: "Pixelated",
    title: "Nearest-neighbor -- best for inspecting raw pixel data",
  },
];

// ─────────────────────────────────────────────────────────────────────────────
// Types
// ─────────────────────────────────────────────────────────────────────────────

export interface ImageViewerProps {
  /** Remote URL or object URL of the image to display. */
  src: string;
  className?: string;
  minScale?: number;
  maxScale?: number;
  /** Show zoom/fit/rotate controls. */
  showToolbar?: boolean;
  /** Show the bottom-right zoom percentage indicator. */
  showZoomIndicator?: boolean;
  /** Show the rotate 90° button in the toolbar. */
  showRotate?: boolean;
  /** Show brightness/contrast/saturation sliders (Konva.Filters.Brighten/
   * Contrast/HSV). Off by default -- caching a node for filters costs an
   * offscreen canvas render, not worth paying for viewers that don't need it. */
  showFilters?: boolean;
  /** Allow dragging the stage to pan. */
  draggable?: boolean;
  /** Called once the image element has loaded with its natural dimensions. */
  onLoad?: (naturalWidth: number, naturalHeight: number) => void;
  /**
   * Additional react-konva <Layer> elements rendered above the base image.
   * Use this for annotation overlays, markers, crop handles, etc. Layers
   * share the Stage's pan/zoom transform, so coordinates line up with the
   * base image's natural pixel space as long as rotation is 0 -- a rotated
   * view only rotates the <KonvaImage> node itself, not sibling layers.
   */
  extraLayers?: React.ReactNode;
  /** Forwarded mouse events from the Konva Stage. */
  onStageMouseDown?: (e: Konva.KonvaEventObject<MouseEvent>) => void;
  onStageMouseMove?: (e: Konva.KonvaEventObject<MouseEvent>) => void;
  onStageMouseUp?: (e: Konva.KonvaEventObject<MouseEvent>) => void;
}

/** Imperative handle for exporting the current image, independent of the
 * viewer's own pan/zoom -- see `ImageViewer`'s `ref`. */
export interface ImageViewerHandle {
  /** Renders the image node as it currently appears -- crop, rotation, and
   * any applied filters (brightness/contrast/saturation) baked in -- to a
   * PNG data URL at its natural pixel size. Returns undefined if the image
   * hasn't loaded yet. */
  exportDataURL(): string | undefined;
}

// ─────────────────────────────────────────────────────────────────────────────
// Small toolbar button
// ─────────────────────────────────────────────────────────────────────────────

function ToolBtn({
  onClick,
  title,
  children,
}: {
  onClick: () => void;
  title: string;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      title={title}
      className="flex h-6 w-6 items-center justify-center rounded text-white transition-colors hover:bg-white/10 hover:text-blue-300"
    >
      {children}
    </button>
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// Filter slider row
// ─────────────────────────────────────────────────────────────────────────────

function FilterSlider({
  label,
  value,
  onChange,
  range,
}: {
  label: string;
  value: number;
  onChange: (value: number) => void;
  range: { min: number; max: number; step: number };
}) {
  return (
    <label className="flex flex-col gap-1 text-[10px] text-white/90">
      <span>{label}</span>
      <input
        type="range"
        min={range.min}
        max={range.max}
        step={range.step}
        value={value}
        onChange={(e) => onChange(Number(e.target.value))}
        className="h-1 w-full accent-brand-500"
      />
    </label>
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// ImageViewer
// ─────────────────────────────────────────────────────────────────────────────

export const ImageViewer = forwardRef<ImageViewerHandle, ImageViewerProps>(
  function ImageViewer(
    {
      src,
      className = "",
      minScale = DEFAULT_MIN_SCALE,
      maxScale = DEFAULT_MAX_SCALE,
      showToolbar = true,
      showZoomIndicator = true,
      showRotate = true,
      showFilters = false,
      draggable = true,
      onLoad,
      extraLayers,
      onStageMouseDown,
      onStageMouseMove,
      onStageMouseUp,
    },
    ref,
  ) {
  const containerRef = useRef<HTMLDivElement>(null);
  const stageRef = useRef<Konva.Stage>(null);
  const imageNodeRef = useRef<Konva.Image>(null);
  const layerRef = useRef<Konva.Layer>(null);

  useImperativeHandle(
    ref,
    () => ({
      exportDataURL: () =>
        imageNodeRef.current?.toDataURL({ mimeType: "image/png" }),
    }),
    [],
  );

  const [stageSize, setStageSize] = useState({ width: 0, height: 0 });
  const [image, setImage] = useState<HTMLImageElement | null>(null);
  const [natural, setNatural] = useState({ w: 1, h: 1 });
  const [pos, setPos] = useState({ x: 0, y: 0 });
  const [scale, setScale] = useState(1);
  const [rotation, setRotation] = useState(0); // 0 | 90 | 180 | 270
  const [isDragging, setIsDragging] = useState(false);
  const [brightness, setBrightness] = useState(0);
  const [contrast, setContrast] = useState(0);
  const [saturation, setSaturation] = useState(0);
  // null = unset -- nothing is forced onto the canvas until the user picks
  // an option; it just renders with whatever the browser does by default.
  const [interpolation, setInterpolation] = useState<Interpolation | null>(
    null,
  );

  // ── Container size tracking ─────────────────────────────────────────────
  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    // Seed from current layout before the observer fires
    const { offsetWidth, offsetHeight } = el;
    if (offsetWidth > 0 && offsetHeight > 0) {
      setStageSize({ width: offsetWidth, height: offsetHeight });
    }
    const ro = new ResizeObserver(([entry]) => {
      const { width, height } = entry.contentRect;
      if (width > 0 && height > 0) setStageSize({ width, height });
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  // ── Load image; reset rotation/filters when src changes ─────────────────
  useEffect(() => {
    setRotation(0);
    setBrightness(0);
    setContrast(0);
    setSaturation(0);
    setInterpolation(null);
    setImage(null);
    const img = new window.Image();
    img.crossOrigin = "anonymous";
    img.src = src;
    img.onload = () => {
      setImage(img);
      setNatural({ w: img.naturalWidth, h: img.naturalHeight });
      onLoad?.(img.naturalWidth, img.naturalHeight);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [src]);

  // ── Fit helper ───────────────────────────────────────────────────────────
  // Rotates image around its center (offsetX/Y = w/2, h/2).
  // Image center in layer coords is always (iw/2, ih/2).
  // Visual effective size after rotation: 90/270 swaps w↔h.
  const fitTo = useCallback(
    (sw: number, sh: number, iw: number, ih: number, rot: number) => {
      if (!sw || !sh || !iw || !ih) return;
      const ew = rot % 180 === 0 ? iw : ih;
      const eh = rot % 180 === 0 ? ih : iw;
      const s = Math.min(sw / ew, sh / eh) * 0.92;
      setScale(s);
      // Center the image center in the stage
      setPos({ x: sw / 2 - (iw / 2) * s, y: sh / 2 - (ih / 2) * s });
    },
    [],
  );

  // Auto-fit when image or stage size becomes available
  useEffect(() => {
    if (image && stageSize.width > 0 && stageSize.height > 0) {
      fitTo(stageSize.width, stageSize.height, natural.w, natural.h, 0);
    }
    // Only re-fit when image or stage size changes, not on rotation
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [image, stageSize.width, stageSize.height]);

  // Konva filters need a cached (rasterized) node to run against. Caching is
  // keyed on the loaded image, not on the filter values themselves --
  // brightness/contrast/saturation are plain node attributes that redraw
  // from the same cache on every change, no re-cache needed.
  useEffect(() => {
    if (!showFilters || !image) return;
    const node = imageNodeRef.current;
    if (!node) return;
    node.cache();
    node.getLayer()?.batchDraw();
  }, [showFilters, image, natural.w, natural.h]);

  // imageSmoothingEnabled has a real Konva attribute (set declaratively on
  // <Layer> below, and kept in sync on every resize by Konva itself), but
  // imageSmoothingQuality doesn't -- it's set directly on the native canvas
  // context here, and re-applied on resize since changing a canvas element's
  // width/height resets all of its 2D context state, quality included.
  // interpolation === null means nothing has been chosen yet, so nothing is
  // forced here -- the canvas just keeps whatever quality it already had.
  useEffect(() => {
    if (interpolation === null) return;
    const ctx = layerRef.current?.getContext()._context;
    if (!ctx) return;
    ctx.imageSmoothingQuality = interpolation === "bicubic" ? "high" : "low";
    layerRef.current?.batchDraw();
  }, [interpolation, stageSize.width, stageSize.height, image]);

  // ── Wheel zoom (cursor-centered) ────────────────────────────────────────
  const handleWheel = useCallback(
    (e: Konva.KonvaEventObject<WheelEvent>) => {
      e.evt.preventDefault();
      const stage = stageRef.current;
      if (!stage) return;
      const pointer = stage.getPointerPosition();
      if (!pointer) return;
      const dir = e.evt.deltaY < 0 ? 1 : -1;
      const newScale = Math.min(
        maxScale,
        Math.max(minScale, scale * (dir > 0 ? ZOOM_FACTOR : 1 / ZOOM_FACTOR)),
      );
      const ratio = newScale / scale;
      setScale(newScale);
      setPos((prev) => ({
        x: pointer.x - (pointer.x - prev.x) * ratio,
        y: pointer.y - (pointer.y - prev.y) * ratio,
      }));
    },
    [scale, minScale, maxScale],
  );

  const handleDragEnd = useCallback((e: Konva.KonvaEventObject<DragEvent>) => {
    setPos({ x: e.target.x(), y: e.target.y() });
    setIsDragging(false);
  }, []);

  // ── Toolbar actions ───────────────────────────────────────────────────────
  const zoomBy = (factor: number) => {
    const newScale = Math.min(maxScale, Math.max(minScale, scale * factor));
    const ratio = newScale / scale;
    const cx = stageSize.width / 2;
    const cy = stageSize.height / 2;
    setScale(newScale);
    setPos((prev) => ({
      x: cx - (cx - prev.x) * ratio,
      y: cy - (cy - prev.y) * ratio,
    }));
  };

  const handleFit = () =>
    fitTo(stageSize.width, stageSize.height, natural.w, natural.h, rotation);

  const handleRotate = () => {
    const newRot = (rotation + 90) % 360;
    setRotation(newRot);
    fitTo(stageSize.width, stageSize.height, natural.w, natural.h, newRot);
  };

  const resetFilters = () => {
    setBrightness(0);
    setContrast(0);
    setSaturation(0);
    setInterpolation(null);
  };
  const filtersAdjusted =
    brightness !== 0 ||
    contrast !== 0 ||
    saturation !== 0 ||
    interpolation !== null;

  // ───────────────────────────────────────────────────────────────────────
  return (
    <div
      ref={containerRef}
      className={`relative overflow-hidden bg-gray-100 dark:bg-gray-900 ${className}`}
    >
      {/* ── Toolbar ── */}
      {showToolbar && (
        <div className="absolute top-2 left-2 z-10 flex items-center gap-0.5 rounded-lg bg-black/50 px-1.5 py-1 backdrop-blur-sm">
          <ToolBtn onClick={() => zoomBy(ZOOM_FACTOR)} title="Zoom in">
            <ZoomInIcon className="h-4 w-4" />
          </ToolBtn>
          <ToolBtn onClick={() => zoomBy(1 / ZOOM_FACTOR)} title="Zoom out">
            <ZoomOutIcon className="h-4 w-4" />
          </ToolBtn>
          <div className="mx-0.5 h-4 w-px bg-white/30" />
          <ToolBtn onClick={handleFit} title="Fit to frame">
            <FitToFrameIcon className="h-4 w-4" />
          </ToolBtn>
          {showRotate && (
            <ToolBtn onClick={handleRotate} title="Rotate 90°">
              <RotateIcon className="h-4 w-4" />
            </ToolBtn>
          )}
        </div>
      )}

      {/* ── Brightness/contrast/saturation ── */}
      {showFilters && (
        <div className="absolute top-2 right-2 z-10 flex w-44 flex-col gap-2 rounded-lg bg-black/50 px-3 py-2.5 backdrop-blur-sm">
          <FilterSlider
            label="Brightness"
            value={brightness}
            onChange={setBrightness}
            range={BRIGHTNESS_RANGE}
          />
          <FilterSlider
            label="Contrast"
            value={contrast}
            onChange={setContrast}
            range={CONTRAST_RANGE}
          />
          <FilterSlider
            label="Saturation"
            value={saturation}
            onChange={setSaturation}
            range={SATURATION_RANGE}
          />

          <div className="flex flex-col gap-1 text-[10px] text-white/90">
            <span>Interpolation</span>
            <div className="flex overflow-hidden rounded-md border border-white/20">
              {INTERPOLATION_OPTIONS.map((option) => (
                <button
                  key={option.value}
                  type="button"
                  onClick={() => setInterpolation(option.value)}
                  title={option.title}
                  className={`flex-1 px-1.5 py-1 text-[10px] transition-colors ${
                    interpolation === option.value
                      ? "bg-brand-500 text-white"
                      : "text-white/70 hover:bg-white/10"
                  }`}
                >
                  {option.label}
                </button>
              ))}
            </div>
          </div>

          <button
            type="button"
            onClick={resetFilters}
            disabled={!filtersAdjusted}
            className="self-end text-[10px] text-white/70 transition-colors hover:text-white disabled:opacity-40"
          >
            Reset
          </button>
        </div>
      )}

      {/* ── Zoom % indicator ── */}
      {showZoomIndicator && (
        <div className="pointer-events-none absolute bottom-2 right-2 z-10 select-none rounded bg-black/40 px-1.5 py-0.5 font-mono text-[10px] text-white">
          {Math.round(scale * 100)}%
        </div>
      )}

      {/* ── Konva Stage ── */}
      {stageSize.width > 0 && (
        <Stage
          ref={stageRef}
          width={stageSize.width}
          height={stageSize.height}
          scaleX={scale}
          scaleY={scale}
          x={pos.x}
          y={pos.y}
          draggable={draggable}
          onWheel={showToolbar ? handleWheel : undefined}
          onDragStart={() => setIsDragging(true)}
          onDragEnd={handleDragEnd}
          onMouseDown={onStageMouseDown}
          onMouseMove={onStageMouseMove}
          onMouseUp={onStageMouseUp}
          style={{ cursor: draggable ? (isDragging ? "grabbing" : "grab") : "default" }}
        >
          <Layer ref={layerRef} imageSmoothingEnabled={interpolation !== "nearest"}>
            {image && (
              <KonvaImage
                ref={imageNodeRef}
                image={image}
                // Rotate around the image center
                x={natural.w / 2}
                y={natural.h / 2}
                offsetX={natural.w / 2}
                offsetY={natural.h / 2}
                rotation={rotation}
                width={natural.w}
                height={natural.h}
                filters={
                  showFilters
                    ? [Konva.Filters.Brighten, Konva.Filters.Contrast, Konva.Filters.HSV]
                    : undefined
                }
                brightness={brightness}
                contrast={contrast}
                saturation={saturation}
                hue={0}
                value={0}
              />
            )}
          </Layer>
          {extraLayers}
        </Stage>
      )}
    </div>
  );
  },
);
