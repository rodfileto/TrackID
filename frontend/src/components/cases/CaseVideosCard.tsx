import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import ComponentCard from "../common/ComponentCard";
import Badge from "../ui/badge/Badge";
import Button from "../ui/button/Button";
import { useCurrentUser } from "../../context/CurrentUserContext";
import VideoPlayer, { type VideoPlayerHandle } from "../video/VideoPlayer";
import PersonFaces from "../video/PersonFaces";
import { personColor, personFacesAt, sameFace, type VideoPerson } from "../video/faces";
import FrameCaptureModal, { type CaptureRequest } from "./FrameCaptureModal";
import {
  clock,
  getVideoObjectUrl,
  listCaseVideos,
  reprocessVideo,
  uploadCaseVideo,
  type CaseVideo,
  type VideoFace,
  type VideoTrack,
} from "../../services/videos";

const buttonClass =
  "rounded-lg border border-gray-300 px-3 py-1.5 text-sm text-gray-700 hover:bg-gray-50 disabled:opacity-50 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-white/5";
const primaryClass =
  "rounded-lg bg-brand-500 px-3 py-1.5 text-sm font-medium text-white hover:bg-brand-600 disabled:opacity-50";

/** The case pipeline samples one frame a second (video.VisionTracker). */
const CASE_INTERVAL_MS = 1000;

/** A track as a person of the shared panel; a video processed before every face was kept has its
 * best face only. */
function personOf(tr: VideoTrack): VideoPerson {
  const best: VideoFace = {
    timestampMs: tr.timestampMs,
    sample: 0,
    box: tr.box,
    confidence: tr.confidence,
    blur: tr.blur,
    embedded: true,
    best: true,
    crop: tr.crop,
  };
  const faces = tr.faces && tr.faces.length > 0 ? tr.faces : [best];
  return { track: tr.track, best: faces.find((f) => f.best) ?? best, faces, firstMs: tr.firstMs, lastMs: tr.lastMs };
}

/** Video sent in as material: uploaded as received (with its SHA-256), processed into one person
 * per track, played here frame by frame, and turned into evidences by capturing frames -- each one
 * the full frame at its time, recording the video, time and frame number for the report, with the
 * faces the examiner chose marked on it as traces. */
export default function CaseVideosCard({
  caseId,
  canWrite,
  onEvidenceAdded,
}: {
  caseId: string;
  /** The user may change the case (holds one of its analyses): upload, capture, process again. */
  canWrite: boolean;
  onEvidenceAdded: () => void;
}) {
  const { t } = useTranslation();
  const { isExaminer } = useCurrentUser();
  const canChange = isExaminer && canWrite;
  const [videos, setVideos] = useState<CaseVideo[]>([]);
  const [busy, setBusy] = useState(false);
  const [openId, setOpenId] = useState<number | null>(null);
  const [capture, setCapture] = useState<CaptureRequest | null>(null);
  const input = useRef<HTMLInputElement>(null);

  const load = useCallback(() => {
    listCaseVideos(caseId).then(setVideos).catch(() => setVideos([]));
  }, [caseId]);
  useEffect(load, [load]);
  // Poll while a video is being processed.
  const working = videos.some((v) => v.status === "PENDING" || v.status === "PROCESSING");
  useEffect(() => {
    if (!working) return;
    const timer = window.setInterval(load, 3000);
    return () => window.clearInterval(timer);
  }, [working, load]);

  async function upload(file: File) {
    setBusy(true);
    try {
      await uploadCaseVideo(caseId, file);
      toast.success(t("videos.uploaded"));
      load();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  async function reprocess(v: CaseVideo) {
    setBusy(true);
    try {
      await reprocessVideo(caseId, v.fileId);
      toast.success(t("videos.reprocessing"));
      load();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  const statusColor = { PENDING: "light", PROCESSING: "info", DONE: "success", FAILED: "error" } as const;
  return (
    <ComponentCard title={t("videos.title")} desc={t("videos.desc")}>
      {videos.length === 0 && <p className="text-sm text-gray-500 dark:text-gray-400">{t("videos.none")}</p>}
      <ul className="space-y-5">
        {videos.map((v) => {
          const oldFormat = v.status === "DONE" && v.tracks.length > 0 && v.tracks.every((tr) => !tr.faces || tr.faces.length === 0);
          return (
            <li key={v.fileId}>
              <div className="flex flex-wrap items-center gap-2">
                <span className="text-sm font-medium text-gray-800 dark:text-white/90">{v.filename}</span>
                <Badge size="sm" color={statusColor[v.status]}>
                  {t(`videos.status.${v.status}`)}
                </Badge>
                {v.durationSeconds !== undefined && v.durationSeconds > 0 && (
                  <span className="text-theme-xs text-gray-500">
                    {clock(v.durationSeconds * 1000)}
                    {v.frameRate ? ` · ${v.frameRate.toFixed(2)} fps` : ""}
                  </span>
                )}
                <span className="font-mono text-theme-xs text-gray-500">{v.sha256.slice(0, 16)}…</span>
                {v.status === "DONE" && (
                  <span className="text-theme-xs text-gray-500">{t("videoLab.found", { count: v.tracks.length })}</span>
                )}
                <span className="ml-auto flex gap-2">
                  {v.status === "DONE" && (
                    <button type="button" className={openId === v.fileId ? buttonClass : primaryClass} onClick={() => setOpenId(openId === v.fileId ? null : v.fileId)}>
                      {openId === v.fileId ? t("videos.close") : t("videos.open")}
                    </button>
                  )}
                  {canChange && (v.status === "FAILED" || oldFormat) && (
                    <button type="button" className={buttonClass} disabled={busy} onClick={() => void reprocess(v)}>
                      {t("videos.reprocess")}
                    </button>
                  )}
                </span>
              </div>
              {v.error && <p className="mt-1 text-sm text-error-500">{v.error}</p>}
              {oldFormat && <p className="mt-1 text-theme-xs text-gray-500">{t("videos.oldFormat")}</p>}
              {v.status === "DONE" && v.tracks.length === 0 && (
                <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">{t("videos.noTracks")}</p>
              )}
              {openId === v.fileId ? (
                <CaseVideoWorkspace caseId={caseId} video={v} canCapture={canChange} onCapture={setCapture} />
              ) : (
                v.tracks.length > 0 && (
                  <div className="mt-3 flex gap-2 overflow-x-auto pb-1">
                    {v.tracks.map((tr) => (
                      <img
                        key={tr.track}
                        src={tr.crop}
                        alt={t("videoLab.person", { n: tr.track + 1 })}
                        title={`${t("videoLab.person", { n: tr.track + 1 })} · ${clock(tr.timestampMs)}`}
                        className="h-14 w-14 shrink-0 rounded border-2 object-cover"
                        style={{ borderColor: personColor(tr.track) }}
                      />
                    ))}
                  </div>
                )
              )}
            </li>
          );
        })}
      </ul>
      {isExaminer && !canWrite && <p className="mt-4 text-theme-xs text-gray-500 dark:text-gray-400">{t("videos.readOnly")}</p>}
      {canChange && (
        <div className="mt-4">
          <input
            ref={input}
            type="file"
            accept="video/*,.mp4,.mov,.avi,.mkv,.webm"
            className="hidden"
            onChange={(e) => {
              const f = e.target.files?.[0];
              if (f) void upload(f);
              e.target.value = "";
            }}
          />
          <Button size="sm" variant="outline" disabled={busy} onClick={() => input.current?.click()}>
            {t("videos.upload")}
          </Button>
        </div>
      )}
      <FrameCaptureModal caseId={caseId} request={capture} onClose={() => setCapture(null)} onCaptured={onEvidenceAdded} />
    </ComponentCard>
  );
}

/** One case video open: the player with "capture this frame", and the persons with their faces. */
function CaseVideoWorkspace({
  caseId,
  video,
  canCapture,
  onCapture,
}: {
  caseId: string;
  video: CaseVideo;
  canCapture: boolean;
  onCapture: (request: CaptureRequest) => void;
}) {
  const { t } = useTranslation();
  const [src, setSrc] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [timeMs, setTimeMs] = useState(0);
  const [selected, setSelected] = useState<{ track: number; face: VideoFace } | null>(null);
  const player = useRef<VideoPlayerHandle>(null);
  const persons = useMemo(() => video.tracks.map(personOf), [video.tracks]);

  useEffect(() => {
    const controller = new AbortController();
    let url: string | null = null;
    getVideoObjectUrl(caseId, video.fileId, controller.signal)
      .then((u) => {
        url = u;
        setSrc(u);
      })
      .catch((err) => !controller.signal.aborted && setError(err instanceof Error ? err.message : String(err)));
    return () => {
      controller.abort();
      if (url) URL.revokeObjectURL(url);
    };
  }, [caseId, video.fileId]);

  const selectFace = useCallback((track: number, face: VideoFace) => {
    setSelected({ track, face });
    player.current?.pause();
    player.current?.seek(face.timestampMs);
  }, []);

  const overlays = useMemo(
    () =>
      personFacesAt(persons, timeMs, CASE_INTERVAL_MS).map(({ track, face }) => ({
        box: face.box,
        color: personColor(track),
        label: `${t("videoLab.person", { n: track + 1 })}${face.best ? " ★" : ""}`,
        dashed: !face.embedded,
        emphasized: !!selected && selected.track === track && sameFace(selected.face, face),
      })),
    [persons, timeMs, selected, t],
  );

  const timeline = useMemo(
    () =>
      persons.map((p) => ({
        key: p.track,
        color: personColor(p.track),
        label: t("videoLab.person", { n: p.track + 1 }),
        points: p.faces.map((f) => ({ ms: f.timestampMs, big: f.best, onClick: () => selectFace(p.track, f) })),
      })),
    [persons, selectFace, t],
  );

  function captureCurrent() {
    const p = player.current;
    if (!p) return;
    p.pause();
    onCapture({ fileId: video.fileId, timestampMs: p.currentFrame().ms });
  }

  function captureFace(track: number, face: VideoFace) {
    player.current?.pause();
    onCapture({ fileId: video.fileId, timestampMs: face.timestampMs, preselect: { track, box: face.box } });
  }

  return (
    <div className="mt-4 space-y-5">
      {error && <p className="text-sm text-error-500">{error}</p>}
      {!src && !error && <p className="py-8 text-center text-sm text-gray-500">{t("videos.loading")}</p>}
      {src && (
        <VideoPlayer
          ref={player}
          src={src}
          fps={video.frameRate}
          onTime={setTimeMs}
          overlays={overlays}
          timeline={timeline}
          actions={
            canCapture && (
              <button type="button" className={primaryClass} onClick={captureCurrent} title={t("videos.captureHintShort")}>
                {t("videos.captureFrame")}
              </button>
            )
          }
        />
      )}
      {persons.length > 0 && (
        <div>
          <h4 className="mb-1 text-sm font-semibold text-gray-800 dark:text-white/90">{t("videoLab.persons")}</h4>
          <p className="mb-3 text-theme-xs text-gray-500 dark:text-gray-400">{t("videos.personsDesc")}</p>
          <PersonFaces
            persons={persons}
            selected={selected}
            onSelect={selectFace}
            onFaceDoubleClick={canCapture ? captureFace : undefined}
            actions={(p) => (
              <>
                <button type="button" className={buttonClass} onClick={() => selectFace(p.track, p.best)}>
                  {t("videoLab.goToBest")}
                </button>
                {canCapture && (
                  <button type="button" className={primaryClass} onClick={() => captureFace(p.track, p.best)}>
                    {t("videos.captureBest")}
                  </button>
                )}
                {canCapture && selected?.track === p.track && !sameFace(selected.face, p.best) && (
                  <button type="button" className={buttonClass} onClick={() => captureFace(p.track, selected.face)}>
                    {t("videos.captureSelected", { time: clock(selected.face.timestampMs) })}
                  </button>
                )}
              </>
            )}
          />
          <p className="mt-3 text-theme-xs text-gray-500 dark:text-gray-400">{t("videoLab.note", { software: "trackid-vision videoproc" })}</p>
        </div>
      )}
    </div>
  );
}
