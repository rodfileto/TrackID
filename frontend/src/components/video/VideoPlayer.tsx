import { forwardRef, useCallback, useEffect, useImperativeHandle, useRef, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { clock } from "../../services/videolab";
import { ASSUMED_FPS } from "./faces";

const buttonClass =
  "rounded-lg border border-gray-300 px-3 py-1.5 text-sm text-gray-700 hover:bg-gray-50 disabled:opacity-50 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-white/5";
const primaryClass =
  "rounded-lg bg-brand-500 px-3 py-1.5 text-sm font-medium text-white hover:bg-brand-600 disabled:opacity-50";

/** A box drawn over the video, in the video's own pixels. */
export interface Overlay {
  box: [number, number, number, number];
  color: string;
  label: string;
  dashed?: boolean;
  emphasized?: boolean;
}

/** A row under the seek bar: where one person appears. */
export interface TimelineRow {
  key: string | number;
  color: string;
  label: string;
  points: { ms: number; big?: boolean; onClick: () => void }[];
}

export interface VideoPlayerHandle {
  seek: (ms: number) => void;
  pause: () => void;
  /** The current time in ms. */
  currentMs: () => number;
  /** The frame shown now and the time to ask the server for exactly that frame (its start). */
  currentFrame: () => { frame: number; ms: number };
}

interface Props {
  src: string;
  fps?: number;
  /** Called as the time changes (each animation frame while playing). */
  onTime?: (ms: number) => void;
  overlays?: Overlay[];
  timeline?: TimelineRow[];
  /** Extra controls after the playback ones (e.g. "Capture this frame"). */
  actions?: ReactNode;
}

/** A video player for examination: frame by frame, 1 s and 10 s steps, normal play at a chosen
 * speed, the time and frame number shown, boxes drawn over the picture and a per-person timeline.
 * Keyboard: Space plays; ←/→ one frame; Shift ±1 s; Alt (or Ctrl) ±10 s. */
const VideoPlayer = forwardRef<VideoPlayerHandle, Props>(function VideoPlayer({ src, fps: fpsProp, onTime, overlays = [], timeline = [], actions }, ref) {
  const { t } = useTranslation();
  const videoRef = useRef<HTMLVideoElement>(null);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const [timeMs, setTimeMsState] = useState(0);
  const [durationMs, setDurationMs] = useState(0);
  const [playing, setPlaying] = useState(false);
  const [rate, setRate] = useState(1);
  const [showBoxes, setShowBoxes] = useState(true);
  const fpsKnown = !!fpsProp && fpsProp > 0;
  const fps = fpsKnown ? fpsProp! : ASSUMED_FPS;

  const onTimeRef = useRef(onTime);
  onTimeRef.current = onTime;
  const setTimeMs = useCallback((ms: number) => {
    setTimeMsState(ms);
    onTimeRef.current?.(ms);
  }, []);

  const seek = useCallback(
    (ms: number) => {
      const v = videoRef.current;
      if (!v) return;
      const max = (v.duration || 0) * 1000;
      const clamped = Math.min(Math.max(0, ms), max > 0 ? max : ms);
      v.currentTime = clamped / 1000;
      setTimeMs(clamped);
    },
    [setTimeMs],
  );

  const frameAt = useCallback((ms: number) => Math.floor((ms / 1000) * fps + 1e-6), [fps]);

  useImperativeHandle(
    ref,
    () => ({
      seek,
      pause: () => videoRef.current?.pause(),
      currentMs: () => (videoRef.current?.currentTime ?? 0) * 1000,
      currentFrame: () => {
        const ms = (videoRef.current?.currentTime ?? 0) * 1000;
        if (!fpsKnown) return { frame: frameAt(ms), ms: Math.round(ms) };
        const frame = frameAt(ms);
        // The frame's own start, rounded down: the server takes the first frame at or after it.
        return { frame, ms: Math.floor((frame * 1000) / fps) };
      },
    }),
    [seek, frameAt, fps, fpsKnown],
  );

  /** Moves by whole frames, landing mid-frame so the browser shows that one. */
  const stepFrames = useCallback(
    (n: number) => {
      const v = videoRef.current;
      if (!v) return;
      v.pause();
      const target = Math.max(0, frameAt(v.currentTime * 1000) + n);
      seek(((target + 0.5) / fps) * 1000);
    },
    [fps, frameAt, seek],
  );

  const stepSeconds = useCallback(
    (s: number) => {
      const v = videoRef.current;
      if (!v) return;
      v.pause();
      seek(v.currentTime * 1000 + s * 1000);
    },
    [seek],
  );

  const togglePlay = useCallback(() => {
    const v = videoRef.current;
    if (!v) return;
    if (v.paused) v.play().catch(() => undefined);
    else v.pause();
  }, []);

  useEffect(() => {
    if (videoRef.current) videoRef.current.playbackRate = rate;
  }, [rate, src]);

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      const target = e.target as HTMLElement | null;
      if (target && (target.tagName === "INPUT" || target.tagName === "TEXTAREA" || target.tagName === "SELECT" || target.isContentEditable)) return;
      if (document.querySelector("[role=dialog], .modal")) return; // a dialog above has the keys
      if (e.key === " ") {
        e.preventDefault();
        togglePlay();
      } else if (e.key === "ArrowLeft" || e.key === "ArrowRight") {
        e.preventDefault();
        const dir = e.key === "ArrowLeft" ? -1 : 1;
        if (e.altKey || e.ctrlKey) stepSeconds(10 * dir);
        else if (e.shiftKey) stepSeconds(dir);
        else stepFrames(dir);
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [togglePlay, stepFrames, stepSeconds]);

  useEffect(() => {
    if (!playing) return;
    let raf = 0;
    const tick = () => {
      if (videoRef.current) setTimeMs(videoRef.current.currentTime * 1000);
      raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  }, [playing, setTimeMs]);

  // Boxes over the picture, which object-fit: contain centres and letterboxes in the element.
  useEffect(() => {
    const v = videoRef.current;
    const canvas = canvasRef.current;
    if (!v || !canvas) return;
    const w = v.clientWidth;
    const h = v.clientHeight;
    const dpr = window.devicePixelRatio || 1;
    canvas.width = Math.round(w * dpr);
    canvas.height = Math.round(h * dpr);
    canvas.style.width = `${w}px`;
    canvas.style.height = `${h}px`;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, w, h);
    if (!showBoxes || !v.videoWidth || !v.videoHeight) return;
    const scale = Math.min(w / v.videoWidth, h / v.videoHeight);
    const ox = (w - v.videoWidth * scale) / 2;
    const oy = (h - v.videoHeight * scale) / 2;
    for (const o of overlays) {
      const [x1, y1, x2, y2] = o.box;
      ctx.strokeStyle = o.color;
      ctx.lineWidth = o.emphasized ? 3 : 2;
      ctx.setLineDash(o.dashed ? [5, 3] : []);
      ctx.strokeRect(ox + x1 * scale, oy + y1 * scale, (x2 - x1) * scale, (y2 - y1) * scale);
      ctx.setLineDash([]);
      ctx.font = "12px sans-serif";
      const tw = ctx.measureText(o.label).width + 6;
      ctx.fillStyle = o.color;
      ctx.fillRect(ox + x1 * scale, oy + y1 * scale - 16, tw, 16);
      ctx.fillStyle = "#fff";
      ctx.fillText(o.label, ox + x1 * scale + 3, oy + y1 * scale - 4);
    }
  }, [overlays, showBoxes, timeMs]);

  return (
    <div>
      <div className="relative mx-auto w-full max-w-4xl bg-black">
        <video
          ref={videoRef}
          src={src}
          className="block max-h-[60vh] w-full object-contain"
          onLoadedMetadata={(e) => setDurationMs(e.currentTarget.duration * 1000)}
          onTimeUpdate={(e) => !playing && setTimeMs(e.currentTarget.currentTime * 1000)}
          onSeeked={(e) => setTimeMs(e.currentTarget.currentTime * 1000)}
          onPlay={() => setPlaying(true)}
          onPause={(e) => {
            setPlaying(false);
            setTimeMs(e.currentTarget.currentTime * 1000);
          }}
          playsInline
          muted
        />
        <canvas ref={canvasRef} className="pointer-events-none absolute left-0 top-0" />
      </div>

      <input
        type="range"
        className="mt-3 w-full"
        min={0}
        max={durationMs || 0}
        step={1}
        value={timeMs}
        onChange={(e) => {
          videoRef.current?.pause();
          seek(Number(e.target.value));
        }}
        aria-label={t("videoLab.position")}
      />

      {timeline.length > 0 && durationMs > 0 && (
        <div className="mt-1 space-y-1">
          {timeline.map((row) => (
            <div key={row.key} className="relative h-3 w-full rounded bg-gray-100 dark:bg-gray-800" title={row.label}>
              {row.points.map((p, i) => (
                <button
                  key={i}
                  type="button"
                  onClick={p.onClick}
                  className="absolute top-0 h-3 -translate-x-1/2 rounded-sm"
                  style={{ left: `${(p.ms / durationMs) * 100}%`, width: p.big ? 6 : 3, background: row.color, opacity: p.big ? 1 : 0.6 }}
                  aria-label={`${row.label} ${clock(p.ms)}`}
                />
              ))}
            </div>
          ))}
        </div>
      )}

      <div className="mt-3 flex flex-wrap items-center justify-center gap-2">
        <button type="button" className={buttonClass} onClick={() => stepSeconds(-10)} title={t("videoLab.keys.back10")}>
          −10 s
        </button>
        <button type="button" className={buttonClass} onClick={() => stepSeconds(-1)} title={t("videoLab.keys.back1")}>
          −1 s
        </button>
        <button type="button" className={buttonClass} onClick={() => stepFrames(-1)} title={t("videoLab.keys.backFrame")}>
          ⟨ {t("videoLab.frame")}
        </button>
        <button type="button" className={primaryClass} onClick={togglePlay} title={t("videoLab.keys.play")}>
          {playing ? t("videoLab.pause") : t("videoLab.play")}
        </button>
        <button type="button" className={buttonClass} onClick={() => stepFrames(1)} title={t("videoLab.keys.fwdFrame")}>
          {t("videoLab.frame")} ⟩
        </button>
        <button type="button" className={buttonClass} onClick={() => stepSeconds(1)} title={t("videoLab.keys.fwd1")}>
          +1 s
        </button>
        <button type="button" className={buttonClass} onClick={() => stepSeconds(10)} title={t("videoLab.keys.fwd10")}>
          +10 s
        </button>
        <select
          className="rounded-lg border border-gray-300 bg-transparent px-2 py-1.5 text-sm text-gray-700 dark:border-gray-700 dark:text-gray-300"
          value={rate}
          onChange={(e) => setRate(Number(e.target.value))}
          aria-label={t("videoLab.speed")}
        >
          {[0.1, 0.25, 0.5, 1, 2, 4].map((r) => (
            <option key={r} value={r}>
              {r}×
            </option>
          ))}
        </select>
        <label className="ml-2 flex items-center gap-1 text-sm text-gray-700 dark:text-gray-300">
          <input type="checkbox" checked={showBoxes} onChange={(e) => setShowBoxes(e.target.checked)} />
          {t("videoLab.showBoxes")}
        </label>
        {actions}
      </div>
      <p className="mt-2 text-center font-mono text-sm text-gray-700 dark:text-gray-300">
        {clock(timeMs)} · {t("videoLab.frameNumber", { n: frameAt(timeMs) })}
        {!fpsKnown && <span className="ml-1 font-sans text-theme-xs text-gray-500">({t("videoLab.fpsAssumed", { fps: ASSUMED_FPS })})</span>}
      </p>
      <p className="mt-1 text-center text-theme-xs text-gray-500 dark:text-gray-400">{t("videoLab.keysHint")}</p>
    </div>
  );
});

export default VideoPlayer;
