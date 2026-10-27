import { useCallback, useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import ComponentCard from "../common/ComponentCard";
import Badge from "../ui/badge/Badge";
import Button from "../ui/button/Button";
import { useCurrentUser } from "../../context/CurrentUserContext";
import { clock, extractVideoFrame, listCaseVideos, uploadCaseVideo, type CaseVideo, type VideoTrack } from "../../services/videos";

/** Video sent in as material: uploaded as received (with its SHA-256), processed into one track
 * per person, and turned into evidences by picking frames -- each one the full frame at its time,
 * recording the video, time and frame number for the report. */
export default function CaseVideosCard({ caseId, onEvidenceAdded }: { caseId: string; onEvidenceAdded: () => void }) {
  const { t } = useTranslation();
  const { isExaminer } = useCurrentUser();
  const [videos, setVideos] = useState<CaseVideo[]>([]);
  const [busy, setBusy] = useState(false);
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

  async function takeFrame(v: CaseVideo, track: VideoTrack) {
    setBusy(true);
    try {
      await extractVideoFrame(caseId, v.fileId, track.timestampMs, track.box);
      toast.success(t("videos.frameAdded", { time: clock(track.timestampMs) }));
      onEvidenceAdded();
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
        {videos.map((v) => (
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
            </div>
            {v.error && <p className="mt-1 text-sm text-error-500">{v.error}</p>}
            {v.status === "DONE" && v.tracks.length === 0 && (
              <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">{t("videos.noTracks")}</p>
            )}
            {v.tracks.length > 0 && (
              <ul className="mt-3 grid grid-cols-2 gap-3 sm:grid-cols-4 lg:grid-cols-6">
                {v.tracks.map((tr) => (
                  <li key={tr.track} className="rounded-lg border border-gray-200 p-2 text-center dark:border-gray-800">
                    <img src={tr.crop} alt={t("videos.track", { n: tr.track + 1 })} className="mx-auto h-24 w-24 rounded object-cover" />
                    <p className="mt-1 text-theme-xs text-gray-700 dark:text-gray-300">{t("videos.track", { n: tr.track + 1 })}</p>
                    <p className="text-theme-xs text-gray-500">
                      {clock(tr.timestampMs)} · {t("videos.detections", { count: tr.detections })}
                    </p>
                    {isExaminer && (
                      <button
                        type="button"
                        disabled={busy}
                        onClick={() => void takeFrame(v, tr)}
                        className="mt-1 text-theme-xs font-medium text-brand-500 hover:underline disabled:opacity-50"
                      >
                        {t("videos.useFrame")}
                      </button>
                    )}
                  </li>
                ))}
              </ul>
            )}
          </li>
        ))}
      </ul>
      {isExaminer && (
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
    </ComponentCard>
  );
}
