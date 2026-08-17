import { useEffect, useMemo, useState } from "react";
import PageBreadcrumb from "../components/common/PageBreadCrumb";
import PageMeta from "../components/common/PageMeta";
import ComponentCard from "../components/common/ComponentCard";
import Label from "../components/form/Label";
import Input from "../components/form/input/InputField";
import FileInput from "../components/form/input/FileInput";
import Checkbox from "../components/form/input/Checkbox";
import Button from "../components/ui/button/Button";
import Alert from "../components/ui/alert/Alert";
import ProgressBar from "../components/ui/progress-bar/ProgressBar";
import {
  Table,
  TableBody,
  TableCell,
  TableHeader,
  TableRow,
} from "../components/ui/table";
import {
  DEFAULT_VIDEO_PROCESSING_PARAMS,
  fetchPersistedVideo,
  fetchVideoList,
  processVideo,
  type VideoJobStatus,
  type VideoProcessingParams,
  type VideoProcessingResponse,
  type VideoSummary,
  type VideoTrack,
} from "../types/videoProcessing";

function formatDuration(seconds: number): string {
  if (seconds < 60) return `${Math.ceil(seconds)}s`;
  const minutes = Math.floor(seconds / 60);
  const remainingSeconds = Math.round(seconds % 60);
  return `${minutes}m ${remainingSeconds}s`;
}

function formatTimestamp(iso: string): string {
  return new Date(iso).toLocaleString(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
  });
}

export default function VideoProcessing() {
  const [file, setFile] = useState<File | null>(null);
  const [params, setParams] = useState<VideoProcessingParams>(
    DEFAULT_VIDEO_PROCESSING_PARAMS,
  );
  const [loading, setLoading] = useState(false);
  const [progress, setProgress] = useState<VideoJobStatus | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<VideoProcessingResponse | null>(null);

  const [videoList, setVideoList] = useState<VideoSummary[]>([]);
  const [totalVideos, setTotalVideos] = useState(0);
  const [page, setPage] = useState(1);
  const pageSize = 5;
  const [loadingList, setLoadingList] = useState(true);
  const [listError, setListError] = useState<string | null>(null);
  const [selectedVideoId, setSelectedVideoId] = useState<string | null>(null);
  const [loadingSelected, setLoadingSelected] = useState(false);

  const totalPages = Math.max(1, Math.ceil(totalVideos / pageSize));

  const videoPreviewUrl = useMemo(
    () => (file ? URL.createObjectURL(file) : null),
    [file],
  );

  const refreshVideoList = async (targetPage: number) => {
    setLoadingList(true);
    setListError(null);
    try {
      const response = await fetchVideoList(targetPage, pageSize);
      setVideoList(response.items);
      setTotalVideos(response.total);
      setPage(response.page);
    } catch (err) {
      setListError(err instanceof Error ? err.message : "Failed to load processed videos.");
    } finally {
      setLoadingList(false);
    }
  };

  useEffect(() => {
    refreshVideoList(page);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [page]);

  const updateParam = <K extends keyof VideoProcessingParams>(
    key: K,
    value: VideoProcessingParams[K],
  ) => {
    setParams((prev) => ({ ...prev, [key]: value }));
  };

  const handleSubmit = async () => {
    if (!file) {
      setError("Select a video file first.");
      return;
    }

    setLoading(true);
    setError(null);
    setResult(null);
    setProgress(null);
    setSelectedVideoId(null);

    try {
      const response = await processVideo(file, params, setProgress);
      setResult(response);
      if (response.video_id) {
        setSelectedVideoId(response.video_id);
      }
      refreshVideoList(1);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to process video.");
    } finally {
      setLoading(false);
    }
  };

  const handleSelectVideo = async (videoId: string) => {
    setSelectedVideoId(videoId);
    setLoadingSelected(true);
    setError(null);
    setFile(null);

    try {
      setResult(await fetchPersistedVideo(videoId));
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load video results.");
      setResult(null);
    } finally {
      setLoadingSelected(false);
    }
  };

  return (
    <div>
      <PageMeta
        title="Video Processing | TrackID"
        description="Upload a video and preview face detection + person clustering results"
      />
      <PageBreadcrumb pageTitle="Video Processing" />

      <div className="grid grid-cols-1 gap-6 xl:grid-cols-3">
        <div className="xl:col-span-1">
          <ComponentCard
            title="Upload video"
            desc="Results are saved to the database, including a crop of every detected face."
          >
            <div>
              <Label>Video file</Label>
              <FileInput
                onChange={(e) => {
                  setFile(e.target.files?.[0] ?? null);
                  setResult(null);
                  setError(null);
                }}
              />
            </div>

            {videoPreviewUrl && (
              <video
                src={videoPreviewUrl}
                controls
                className="w-full rounded-lg border border-gray-200 dark:border-gray-800"
              />
            )}

            <div className="grid grid-cols-2 gap-4">
              <div>
                <Label>Sample interval (s)</Label>
                <Input
                  type="number"
                  step={0.1}
                  min="0.1"
                  value={params.interval_seconds}
                  onChange={(e) =>
                    updateParam("interval_seconds", Number(e.target.value))
                  }
                />
              </div>
              <div>
                <Label>Embed interval (s)</Label>
                <Input
                  type="number"
                  step={0.1}
                  min="0.1"
                  value={params.embed_interval_seconds}
                  onChange={(e) =>
                    updateParam("embed_interval_seconds", Number(e.target.value))
                  }
                />
              </div>
              <div>
                <Label>Min quality</Label>
                <Input
                  type="number"
                  step={1}
                  min="0"
                  value={params.min_quality}
                  onChange={(e) =>
                    updateParam("min_quality", Number(e.target.value))
                  }
                />
              </div>
              <div>
                <Label>Min blur</Label>
                <Input
                  type="number"
                  step={1}
                  min="0"
                  value={params.min_blur}
                  onChange={(e) =>
                    updateParam("min_blur", Number(e.target.value))
                  }
                />
              </div>
              <div>
                <Label>IoU threshold</Label>
                <Input
                  type="number"
                  step={0.05}
                  min="0"
                  max="1"
                  value={params.iou_threshold}
                  onChange={(e) =>
                    updateParam("iou_threshold", Number(e.target.value))
                  }
                />
              </div>
              <div>
                <Label>Cluster eps</Label>
                <Input
                  type="number"
                  step={0.05}
                  min="0"
                  value={params.cluster_eps}
                  onChange={(e) =>
                    updateParam("cluster_eps", Number(e.target.value))
                  }
                />
              </div>
              <div>
                <Label>Cluster min samples</Label>
                <Input
                  type="number"
                  step={1}
                  min="1"
                  value={params.cluster_min_samples}
                  onChange={(e) =>
                    updateParam("cluster_min_samples", Number(e.target.value))
                  }
                />
              </div>
            </div>

            <Checkbox
              label="Full detection on every sampled frame (slower, more accurate)"
              checked={params.full_detection_every_frame}
              onChange={(checked) =>
                updateParam("full_detection_every_frame", checked)
              }
            />
            <Checkbox
              label="Include best-face crop in results"
              checked={params.include_crops}
              onChange={(checked) => updateParam("include_crops", checked)}
            />

            <Button onClick={handleSubmit} disabled={loading || !file}>
              {loading ? "Processing..." : "Process video"}
            </Button>

            {loading && (
              <div>
                <ProgressBar
                  percent={progress?.percent ?? 0}
                  label={
                    progress
                      ? `Frame ${progress.frame_count} / ${progress.expected_frames}`
                      : "Starting..."
                  }
                />
                <p className="mt-1.5 text-sm text-gray-500 dark:text-gray-400">
                  {progress && progress.eta_seconds !== null
                    ? `~${formatDuration(progress.eta_seconds)} remaining`
                    : "Sampling frames, detecting faces, and clustering — this can take a while for longer videos."}
                </p>
              </div>
            )}

            {error && (
              <Alert variant="error" title="Processing failed" message={error} />
            )}
          </ComponentCard>

          <ComponentCard
            title="Processed videos"
            desc="Previously processed videos, loaded from the database. Click a row to view its results."
          >
            {loadingList ? (
              <p className="text-sm text-gray-500 dark:text-gray-400">Loading…</p>
            ) : listError ? (
              <Alert variant="error" title="Failed to load videos" message={listError} />
            ) : videoList.length === 0 ? (
              <p className="text-sm text-gray-500 dark:text-gray-400">
                No videos processed yet.
              </p>
            ) : (
              <div className="overflow-hidden rounded-xl border border-gray-200 dark:border-white/[0.05]">
                <div className="max-w-full overflow-x-auto">
                  <Table>
                    <TableHeader className="border-b border-gray-100 dark:border-white/[0.05]">
                      <TableRow>
                        <TableCell
                          isHeader
                          className="px-4 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
                        >
                          Filename
                        </TableCell>
                        <TableCell
                          isHeader
                          className="px-4 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
                        >
                          Processed
                        </TableCell>
                        <TableCell
                          isHeader
                          className="px-4 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
                        >
                          People
                        </TableCell>
                        <TableCell
                          isHeader
                          className="px-4 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
                        >
                          Detections
                        </TableCell>
                      </TableRow>
                    </TableHeader>
                    <TableBody className="divide-y divide-gray-100 dark:divide-white/[0.05]">
                      {videoList.map((video) => (
                        <TableRow
                          key={video.video_id}
                          onClick={() => handleSelectVideo(video.video_id)}
                          className={`cursor-pointer hover:bg-gray-50 dark:hover:bg-white/[0.03] ${
                            selectedVideoId === video.video_id
                              ? "bg-brand-50 dark:bg-brand-500/10"
                              : ""
                          }`}
                        >
                          <TableCell className="px-4 py-3 text-gray-700 text-start text-theme-sm dark:text-gray-300">
                            {video.original_filename ?? "Untitled video"}
                          </TableCell>
                          <TableCell className="px-4 py-3 text-gray-500 text-start text-theme-sm dark:text-gray-400">
                            {formatTimestamp(video.created_at)}
                          </TableCell>
                          <TableCell className="px-4 py-3 text-gray-500 text-start text-theme-sm dark:text-gray-400">
                            {video.track_count}
                          </TableCell>
                          <TableCell className="px-4 py-3 text-gray-500 text-start text-theme-sm dark:text-gray-400">
                            {video.total_detections}
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </div>

                <div className="flex items-center justify-between border-t border-gray-100 px-4 py-3 dark:border-white/[0.05]">
                  <p className="text-xs text-gray-500 dark:text-gray-400">
                    Page {page} of {totalPages} · {totalVideos} video
                    {totalVideos === 1 ? "" : "s"}
                  </p>
                  <div className="flex gap-2">
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={page <= 1}
                      onClick={() => setPage((p) => p - 1)}
                    >
                      Prev
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={page >= totalPages}
                      onClick={() => setPage((p) => p + 1)}
                    >
                      Next
                    </Button>
                  </div>
                </div>
              </div>
            )}
          </ComponentCard>
        </div>

        <div className="xl:col-span-2">
          {loadingSelected ? (
            <ComponentCard title="Loading results…">
              <p className="text-sm text-gray-500 dark:text-gray-400">
                Fetching persisted results…
              </p>
            </ComponentCard>
          ) : (
            result && (
              <ComponentCard
                title={`Results — ${result.track_count} ${
                  result.track_count === 1 ? "person" : "people"
                } detected`}
              >
                {result.track_count === 0 ? (
                  <p className="text-sm text-gray-500 dark:text-gray-400">
                    No clustered faces found. Try lowering "min blur" / "min
                    quality", or shortening the sample interval.
                  </p>
                ) : (
                  <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                    {result.tracks.map((track) => (
                      <TrackCard key={track.track_id} track={track} />
                    ))}
                  </div>
                )}
              </ComponentCard>
            )
          )}
        </div>
      </div>
    </div>
  );
}

function TrackCard({ track }: { track: VideoTrack }) {
  const [expanded, setExpanded] = useState(false);
  const { best_face, all_faces } = track;

  return (
    <div className="rounded-xl border border-gray-200 p-4 dark:border-gray-800">
      <div className="flex gap-4">
        {best_face.face_crop ? (
          <img
            src={best_face.face_crop}
            alt={`Track ${track.track_id} best face`}
            className="h-24 w-24 rounded-lg object-cover"
          />
        ) : (
          <div className="flex h-24 w-24 items-center justify-center rounded-lg bg-gray-100 text-xs text-gray-400 dark:bg-white/[0.03]">
            No crop
          </div>
        )}

        <div className="flex-1 text-sm">
          <p className="font-medium text-gray-800 dark:text-white/90">
            Person #{track.track_id}
          </p>
          <p className="text-gray-500 dark:text-gray-400">
            {all_faces.length} detection{all_faces.length === 1 ? "" : "s"}
          </p>
          <dl className="mt-1 grid grid-cols-2 gap-x-2 text-gray-500 dark:text-gray-400">
            <dt>Quality</dt>
            <dd>{best_face.quality_score?.toFixed(1) ?? "—"}</dd>
            <dt>Confidence</dt>
            <dd>{(best_face.confidence * 100).toFixed(0)}%</dd>
            <dt>Age / Gender</dt>
            <dd>
              {best_face.estimated_age ?? "—"} / {best_face.estimated_gender ?? "—"}
            </dd>
            <dt>Frame</dt>
            <dd>
              {best_face.frame_number} ({best_face.timestamp_seconds.toFixed(1)}s)
            </dd>
          </dl>
        </div>
      </div>

      <button
        type="button"
        className="mt-3 text-sm font-medium text-brand-500 hover:text-brand-600"
        onClick={() => setExpanded((prev) => !prev)}
      >
        {expanded ? "Hide" : "Show"} all detections
      </button>

      {expanded && (
        <div className="mt-2 max-h-48 overflow-y-auto rounded-lg border border-gray-100 dark:border-gray-800">
          <table className="w-full text-left text-xs">
            <thead className="sticky top-0 bg-gray-50 text-gray-500 dark:bg-gray-900 dark:text-gray-400">
              <tr>
                <th className="px-2 py-1">Crop</th>
                <th className="px-2 py-1">Frame</th>
                <th className="px-2 py-1">Time (s)</th>
                <th className="px-2 py-1">Conf.</th>
                <th className="px-2 py-1">Quality</th>
                <th className="px-2 py-1">Blur</th>
                <th className="px-2 py-1">Type</th>
              </tr>
            </thead>
            <tbody>
              {all_faces.map((det, i) => (
                <tr
                  key={i}
                  className="border-t border-gray-100 text-gray-600 dark:border-gray-800 dark:text-gray-300"
                >
                  <td className="px-2 py-1">
                    {det.face_crop ? (
                      <img
                        src={det.face_crop}
                        alt={`Frame ${det.frame_number} crop`}
                        className="h-8 w-8 rounded object-cover"
                      />
                    ) : (
                      "—"
                    )}
                  </td>
                  <td className="px-2 py-1">{det.frame_number}</td>
                  <td className="px-2 py-1">{det.timestamp_seconds.toFixed(1)}</td>
                  <td className="px-2 py-1">{(det.confidence * 100).toFixed(0)}%</td>
                  <td className="px-2 py-1">{det.quality_score?.toFixed(1) ?? "—"}</td>
                  <td className="px-2 py-1">{det.blur_score?.toFixed(0) ?? "—"}</td>
                  <td className="px-2 py-1">
                    {det.is_embedding ? "full" : "light"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
