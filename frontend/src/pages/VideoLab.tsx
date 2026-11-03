import { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import PageMeta from "../components/common/PageMeta";
import PageBreadcrumb from "../components/common/PageBreadCrumb";
import ComponentCard from "../components/common/ComponentCard";
import Badge from "../components/ui/badge/Badge";
import VideoPlayer, { type VideoPlayerHandle } from "../components/video/VideoPlayer";
import PersonFaces from "../components/video/PersonFaces";
import { personColor, personFacesAt, sameFace } from "../components/video/faces";
import {
  clock,
  deleteVideoLabJob,
  getVideoLabFrame,
  getVideoLabInfo,
  getVideoLabJob,
  uploadVideoLab,
  VideoLabExpired,
  type VideoLabFace,
  type VideoLabInfo,
  type VideoLabJob,
} from "../services/videolab";

const buttonClass =
  "rounded-lg border border-gray-300 px-3 py-1.5 text-sm text-gray-700 hover:bg-gray-50 disabled:opacity-50 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-white/5";
const primaryClass =
  "rounded-lg bg-brand-500 px-3 py-1.5 text-sm font-medium text-white hover:bg-brand-600 disabled:opacity-50";
const inputClass =
  "w-24 rounded-lg border border-gray-300 bg-transparent px-2 py-1 text-sm text-gray-800 dark:border-gray-700 dark:text-white/90";

/** A selected face: the person's track and the face. */
type Selection = { track: number; face: VideoLabFace };

function downloadBlob(name: string, blob: Blob) {
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  a.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

export default function VideoLab() {
  const { t } = useTranslation();
  const [info, setInfo] = useState<VideoLabInfo | null>(null);
  const [file, setFile] = useState<File | null>(null);
  const [src, setSrc] = useState<string | null>(null);
  const [interval, setIntervalSeconds] = useState(1);
  const [minBlur, setMinBlur] = useState(30);
  const [job, setJob] = useState<VideoLabJob | null>(null);
  const [sending, setSending] = useState(false);
  const [selected, setSelected] = useState<Selection | null>(null);
  const player = useRef<VideoPlayerHandle>(null);
  const [timeMs, setTimeMs] = useState(0);

  useEffect(() => {
    getVideoLabInfo()
      .then((i) => {
        setInfo(i);
        setIntervalSeconds(i.defaultInterval);
        setMinBlur(i.defaultMinBlur);
      })
      .catch((err) => toast.error(err instanceof Error ? err.message : String(err)));
  }, []);

  // The local copy plays in the browser; the server only finds faces and extracts full frames.
  useEffect(() => {
    if (!file) return;
    const url = URL.createObjectURL(file);
    setSrc(url);
    return () => URL.revokeObjectURL(url);
  }, [file]);

  // Remove the video from the server when leaving the page or sending another.
  const jobId = job?.id;
  useEffect(() => {
    if (!jobId) return;
    return () => {
      deleteVideoLabJob(jobId).catch(() => undefined);
    };
  }, [jobId]);

  // Poll while the server works.
  const working = job?.status === "PENDING" || job?.status === "PROCESSING";
  useEffect(() => {
    if (!jobId || !working) return;
    const controller = new AbortController();
    const id = setInterval(() => {
      getVideoLabJob(jobId, controller.signal)
        .then(setJob)
        .catch((err) => {
          if (controller.signal.aborted) return;
          if (err instanceof VideoLabExpired) setJob(null);
          toast.error(err instanceof Error ? err.message : String(err));
        });
    }, 1000);
    return () => {
      clearInterval(id);
      controller.abort();
    };
  }, [jobId, working]);

  const persons = useMemo(() => job?.persons ?? [], [job]);
  const intervalMs = (job?.options.intervalSeconds ?? interval) * 1000;

  function chooseFile(f: File | null) {
    setFile(f);
    setJob(null);
    setSelected(null);
    setTimeMs(0);
  }

  async function process() {
    if (!file) return;
    setSending(true);
    setSelected(null);
    try {
      setJob(await uploadVideoLab(file, interval, minBlur));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : String(err));
    } finally {
      setSending(false);
    }
  }

  const overlays = useMemo(
    () =>
      personFacesAt(persons, timeMs, intervalMs).map(({ track, face }) => ({
        box: face.box,
        color: personColor(track),
        label: `${t("videoLab.person", { n: track + 1 })}${face.best ? " ★" : ""}`,
        dashed: !face.embedded,
        emphasized: !!selected && selected.track === track && sameFace(selected.face, face),
      })),
    [persons, timeMs, intervalMs, selected, t],
  );

  const timeline = useMemo(
    () =>
      persons.map((p) => ({
        key: p.track,
        color: personColor(p.track),
        label: t("videoLab.person", { n: p.track + 1 }),
        points: p.faces.map((f) => ({ ms: f.timestampMs, big: f.best, onClick: () => selectFace(p.track, f) })),
      })),
    [persons, t],
  );

  function selectFace(track: number, face: VideoLabFace) {
    setSelected({ track, face });
    player.current?.pause();
    player.current?.seek(face.timestampMs);
  }

  async function saveFullFrame(face: VideoLabFace) {
    if (!job) return;
    try {
      const blob = await getVideoLabFrame(job.id, face.timestampMs);
      const base = job.filename.replace(/\.[^.]+$/, "");
      downloadBlob(`${base} @ ${clock(face.timestampMs).replace(/:/g, "-")}.png`, blob);
    } catch (err) {
      if (err instanceof VideoLabExpired) setJob(null);
      toast.error(err instanceof Error ? err.message : String(err));
    }
  }

  const progress = job && job.expectedFrames > 0 ? Math.min(100, Math.round((job.frames / job.expectedFrames) * 100)) : 0;

  return (
    <>
      <PageMeta title={t("videoLab.title")} description={t("videoLab.desc")} />
      <PageBreadcrumb pageTitle={t("videoLab.title")} />
      <div className="space-y-6">
        <ComponentCard title={t("videoLab.title")} desc={t("videoLab.desc")}>
          <div className="flex flex-wrap items-end gap-4">
            <label className="flex flex-col gap-1 text-sm text-gray-700 dark:text-gray-300">
              {t("videoLab.file")}
              <input
                type="file"
                accept="video/*,.mp4,.m4v,.mov,.avi,.mkv,.webm"
                onChange={(e) => chooseFile(e.target.files?.[0] ?? null)}
                className="text-sm"
              />
            </label>
            <label className="flex flex-col gap-1 text-sm text-gray-700 dark:text-gray-300">
              {t("videoLab.interval")}
              <input
                type="number"
                className={inputClass}
                min={info?.minInterval ?? 0.1}
                max={info?.maxInterval ?? 10}
                step={0.1}
                value={interval}
                onChange={(e) => setIntervalSeconds(Number(e.target.value))}
              />
            </label>
            <label className="flex flex-col gap-1 text-sm text-gray-700 dark:text-gray-300">
              {t("videoLab.minBlur")}
              <input
                type="number"
                className={inputClass}
                min={0}
                step={5}
                value={minBlur}
                onChange={(e) => setMinBlur(Number(e.target.value))}
              />
            </label>
            <button type="button" className={primaryClass} disabled={!file || sending || working || info?.enabled === false} onClick={process}>
              {sending ? t("videoLab.sending") : t("videoLab.process")}
            </button>
            {info && !info.enabled && <Badge color="warning">{t("videoLab.disabled")}</Badge>}
          </div>
          <p className="mt-2 text-theme-xs text-gray-500 dark:text-gray-400">{t("videoLab.hint")}</p>
          {job && (
            <div className="mt-3 text-sm text-gray-700 dark:text-gray-300">
              <div className="flex flex-wrap items-center gap-2">
                <Badge color={job.status === "DONE" ? "success" : job.status === "FAILED" ? "error" : "info"}>
                  {t(`videoLab.status.${job.status}`)}
                </Badge>
                <span className="font-mono text-theme-xs">SHA-256 {job.sha256.slice(0, 16)}…</span>
                {job.durationSeconds ? <span>{clock(job.durationSeconds * 1000)}</span> : null}
                {job.frameRate ? <span>{job.frameRate.toFixed(3)} fps</span> : null}
                {job.status === "DONE" && <span>{t("videoLab.found", { count: persons.length })}</span>}
              </div>
              {working && (
                <div className="mt-2">
                  <div className="h-2 w-full overflow-hidden rounded bg-gray-200 dark:bg-gray-800">
                    <div className="h-full bg-brand-500 transition-all" style={{ width: `${progress}%` }} />
                  </div>
                  <p className="mt-1 text-theme-xs text-gray-500">
                    {t("videoLab.progress", { frames: job.frames, expected: job.expectedFrames })}
                  </p>
                </div>
              )}
              {job.error && <p className="mt-2 text-sm text-error-600">{job.error}</p>}
            </div>
          )}
        </ComponentCard>

        {src && (
          <ComponentCard title={t("videoLab.player")}>
            <VideoPlayer ref={player} src={src} fps={job?.frameRate} onTime={setTimeMs} overlays={overlays} timeline={timeline} />
          </ComponentCard>
        )}

        {job?.status === "DONE" && (
          <ComponentCard title={t("videoLab.persons")} desc={t("videoLab.personsDesc")}>
            {persons.length === 0 && <p className="text-sm text-gray-500 dark:text-gray-400">{t("videoLab.none")}</p>}
            <PersonFaces
              persons={persons}
              selected={selected}
              onSelect={selectFace}
              onFaceDoubleClick={(_, f) => void saveFullFrame(f)}
              actions={(p) => (
                <>
                  <button type="button" className={buttonClass} onClick={() => selectFace(p.track, p.best)}>
                    {t("videoLab.goToBest")}
                  </button>
                  <button type="button" className={buttonClass} onClick={() => void saveFullFrame(p.best)}>
                    {t("videoLab.saveFrame")}
                  </button>
                </>
              )}
            />
            <p className="mt-4 text-theme-xs text-gray-500 dark:text-gray-400">{t("videoLab.note", { software: job.software })}</p>
          </ComponentCard>
        )}
      </div>
    </>
  );
}
