import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { Modal } from "../ui/modal";
import { personColor, iou } from "../video/faces";
import { clock, extractVideoFrame, getFrameFaces, type VideoFrame } from "../../services/videos";

const buttonClass =
  "rounded-lg border border-gray-300 px-3 py-1.5 text-sm text-gray-700 hover:bg-gray-50 disabled:opacity-50 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-white/5";
const primaryClass =
  "rounded-lg bg-brand-500 px-3 py-1.5 text-sm font-medium text-white hover:bg-brand-600 disabled:opacity-50";

/** Where a capture starts: a time in the video and, from a person's face, that face to preselect. */
export interface CaptureRequest {
  fileId: number;
  timestampMs: number;
  preselect?: { track: number; box: [number, number, number, number] };
}

/** One frame of a case's video with the faces found in it: the examiner chooses one, several or
 * all, and the frame becomes an evidence with each chosen face marked as a trace. */
export default function FrameCaptureModal({
  caseId,
  request,
  onClose,
  onCaptured,
}: {
  caseId: string;
  request: CaptureRequest | null;
  onClose: () => void;
  onCaptured: () => void;
}) {
  const { t } = useTranslation();
  const [frame, setFrame] = useState<VideoFrame | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [chosen, setChosen] = useState<Set<number>>(new Set());
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!request) return;
    let cancelled = false;
    setFrame(null);
    setError(null);
    setChosen(new Set());
    getFrameFaces(caseId, request.fileId, request.timestampMs)
      .then((f) => {
        if (cancelled) return;
        setFrame(f);
        // From a person's face: that face (the detection overlapping it most). From the player:
        // the only face when there is one; otherwise the examiner chooses.
        const pre = new Set<number>();
        if (request.preselect) {
          let best = -1;
          let bestScore = 0.3;
          f.faces.forEach((face, i) => {
            const score = iou(face.box, request.preselect!.box) + (face.track === request.preselect!.track ? 0.5 : 0);
            if (score > bestScore) {
              best = i;
              bestScore = score;
            }
          });
          if (best >= 0) pre.add(best);
        } else if (f.faces.length === 1) {
          pre.add(0);
        }
        setChosen(pre);
      })
      .catch((err) => !cancelled && setError(err instanceof Error ? err.message : String(err)));
    return () => {
      cancelled = true;
    };
  }, [caseId, request]);

  function toggle(i: number) {
    setChosen((prev) => {
      const next = new Set(prev);
      if (next.has(i)) next.delete(i);
      else next.add(i);
      return next;
    });
  }

  async function save(withFaces: boolean) {
    if (!request || !frame) return;
    setSaving(true);
    try {
      const faces = withFaces ? frame.faces.filter((_, i) => chosen.has(i)).map((f) => ({ box: f.box, score: f.score })) : [];
      const out = await extractVideoFrame(caseId, request.fileId, frame.timestampMs, faces);
      const time = clock(frame.timestampMs);
      if (out.existing) toast.success(t("videos.capturedExisting", { count: out.traces.length, time }));
      else toast.success(t("videos.captured", { count: faces.length, time }));
      onCaptured();
      onClose();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : String(err));
    } finally {
      setSaving(false);
    }
  }

  const faceLabel = (i: number) => {
    const track = frame?.faces[i].track;
    return track !== undefined ? `${i + 1} · ${t("videoLab.person", { n: track + 1 })}` : `${i + 1}`;
  };
  const faceColor = (i: number) => {
    const track = frame?.faces[i].track;
    return track !== undefined ? personColor(track) : "#64748b";
  };

  return (
    <Modal isOpen={request !== null} onClose={onClose} className="m-4 max-w-5xl">
      <div className="p-5 sm:p-6">
        <h3 className="text-lg font-semibold text-gray-800 dark:text-white/90">{t("videos.captureTitle")}</h3>
        <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
          {request && clock(frame?.timestampMs ?? request.timestampMs)}
          {frame?.frame !== undefined && ` · ${t("videoLab.frameNumber", { n: frame.frame })}`}
          {frame && ` · ${frame.width}×${frame.height}`}
        </p>

        {error && <p className="mt-4 text-sm text-error-500">{error}</p>}
        {!frame && !error && <p className="py-16 text-center text-sm text-gray-500">{t("videos.findingFaces")}</p>}

        {frame && (
          <>
            <div className="relative mt-4 bg-black">
              <img src={frame.image} alt={clock(frame.timestampMs)} className="block max-h-[60vh] w-full object-contain" />
              {/* Same box as the image, so the SVG's viewBox maps onto the frame's pixels. */}
              <svg
                className="absolute inset-0 h-full w-full"
                viewBox={`0 0 ${frame.width} ${frame.height}`}
                preserveAspectRatio="xMidYMid meet"
              >
                {frame.faces.map((f, i) => {
                  const [x1, y1, x2, y2] = f.box;
                  const on = chosen.has(i);
                  const stroke = Math.max(2, frame.width / 400);
                  return (
                    <g key={i} className="cursor-pointer" onClick={() => toggle(i)}>
                      <rect
                        x={x1}
                        y={y1}
                        width={x2 - x1}
                        height={y2 - y1}
                        fill={on ? "rgba(255,255,255,0.12)" : "transparent"}
                        stroke={faceColor(i)}
                        strokeWidth={on ? stroke * 2 : stroke}
                        strokeDasharray={on ? undefined : `${stroke * 3} ${stroke * 2}`}
                      />
                      <text x={x1 + stroke} y={y1 - stroke * 2} fill={faceColor(i)} fontSize={Math.max(14, frame.width / 60)} fontWeight="bold">
                        {on ? "✓ " : ""}
                        {faceLabel(i)}
                      </text>
                    </g>
                  );
                })}
              </svg>
            </div>

            {!frame.detected && <p className="mt-2 text-theme-xs text-warning-600">{t("videos.notDetected")}</p>}
            {frame.faces.length === 0 ? (
              <p className="mt-3 text-sm text-gray-500 dark:text-gray-400">{t("videos.noFacesInFrame")}</p>
            ) : (
              <div className="mt-3 flex flex-wrap items-center gap-3">
                {frame.faces.map((f, i) => (
                  <label key={i} className="flex items-center gap-1.5 text-sm text-gray-700 dark:text-gray-300">
                    <input type="checkbox" checked={chosen.has(i)} onChange={() => toggle(i)} />
                    <span style={{ color: faceColor(i) }} className="font-medium">
                      {faceLabel(i)}
                    </span>
                    <span className="text-theme-xs text-gray-500">{f.score.toFixed(2)}</span>
                  </label>
                ))}
                <button type="button" className={buttonClass} onClick={() => setChosen(new Set(frame.faces.map((_, i) => i)))}>
                  {t("videos.selectAll")}
                </button>
                <button type="button" className={buttonClass} onClick={() => setChosen(new Set())}>
                  {t("videos.selectNone")}
                </button>
              </div>
            )}
            <p className="mt-2 text-theme-xs text-gray-500 dark:text-gray-400">{t("videos.captureHint")}</p>

            <div className="mt-5 flex flex-wrap justify-end gap-2">
              <button type="button" className={buttonClass} onClick={onClose} disabled={saving}>
                {t("common.cancel")}
              </button>
              <button type="button" className={buttonClass} onClick={() => void save(false)} disabled={saving}>
                {t("videos.frameOnly")}
              </button>
              <button type="button" className={primaryClass} onClick={() => void save(true)} disabled={saving || chosen.size === 0}>
                {t("videos.captureFaces", { count: chosen.size })}
              </button>
            </div>
          </>
        )}
      </div>
    </Modal>
  );
}
