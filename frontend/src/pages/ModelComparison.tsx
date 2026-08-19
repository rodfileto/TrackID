import { useEffect, useState } from "react";
import PageBreadcrumb from "../components/common/PageBreadCrumb";
import PageMeta from "../components/common/PageMeta";
import ComponentCard from "../components/common/ComponentCard";
import Alert from "../components/ui/alert/Alert";
import type { VideoTrack } from "../types/videoProcessing";

interface ComparisonResult {
  engine: string;
  elapsed_seconds: number;
  tracks: VideoTrack[];
  track_count: number;
}

async function fetchComparisonResult(path: string): Promise<ComparisonResult> {
  const response = await fetch(path);
  if (!response.ok) {
    throw new Error(`Failed to load ${path} (${response.status})`);
  }
  return response.json();
}

export default function ModelComparison() {
  const [insightface, setInsightface] = useState<ComparisonResult | null>(null);
  const [adaface, setAdaface] = useState<ComparisonResult | null>(null);
  const [auraface, setAuraface] = useState<ComparisonResult | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    Promise.all([
      fetchComparisonResult("/comparison_insightface.json"),
      fetchComparisonResult("/comparison_adaface.json"),
      fetchComparisonResult("/comparison_auraface.json"),
    ])
      .then(([insightfaceResult, adafaceResult, aurafaceResult]) => {
        setInsightface(insightfaceResult);
        setAdaface(adafaceResult);
        setAuraface(aurafaceResult);
      })
      .catch((err) =>
        setError(err instanceof Error ? err.message : "Failed to load comparison results."),
      )
      .finally(() => setLoading(false));
  }, []);

  return (
    <div>
      <PageMeta
        title="Model Comparison | TrackID"
        description="InsightFace (ArcFace) vs AdaFace face-embedding comparison on the same video"
      />
      <PageBreadcrumb pageTitle="Model Comparison" />

      <ComponentCard
        title="InsightFace (ArcFace) vs AdaFace (IR101) vs AuraFace (glintr100)"
        desc="Ground truth for this video: 8 people. All three now hit exactly 8, using the same SCRFD detector, min_samples=2, and min_blur=30. Two fixes got here: min_samples=1 (the original setting) let single stray detections form their own 'person' — switching to 2 alone cut inflated counts roughly in half. The remaining fragmentation was mostly blurry frames producing embeddings that didn't match their own person's cleaner frames closely enough — filtering those out (min_blur=30) resolved it for all three without needing per-model eps gymnastics. eps: InsightFace/AdaFace 0.57/0.625 (ArcFace-family embedding space), AuraFace 0.55 (its own, more separated space)."
      >
        {loading && (
          <p className="text-sm text-gray-500 dark:text-gray-400">Loading comparison results…</p>
        )}
        {error && <Alert variant="error" title="Failed to load results" message={error} />}
      </ComponentCard>

      {insightface && adaface && auraface && (
        <div className="mt-6 grid grid-cols-1 gap-6 xl:grid-cols-3">
          <EngineColumn result={insightface} label="InsightFace — buffalo_l (ArcFace)" />
          <EngineColumn result={adaface} label="AdaFace — IR101 (WebFace4M)" />
          <EngineColumn result={auraface} label="AuraFace — glintr100 (Apache 2.0)" />
        </div>
      )}
    </div>
  );
}

function EngineColumn({ result, label }: { result: ComparisonResult; label: string }) {
  return (
    <ComponentCard
      title={`${label} — ${result.track_count} ${result.track_count === 1 ? "person" : "people"}`}
      desc={`Processed in ${result.elapsed_seconds.toFixed(1)}s`}
    >
      {result.track_count === 0 ? (
        <p className="text-sm text-gray-500 dark:text-gray-400">No clustered faces found.</p>
      ) : (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          {result.tracks.map((track) => (
            <TrackCard key={track.track_id} track={track} />
          ))}
        </div>
      )}
    </ComponentCard>
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
            <dt>Blur</dt>
            <dd>{best_face.blur_score?.toFixed(0) ?? "—"}</dd>
            <dt>Confidence</dt>
            <dd>{(best_face.confidence * 100).toFixed(0)}%</dd>
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
                <th className="px-2 py-1">Blur</th>
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
                  <td className="px-2 py-1">{det.blur_score?.toFixed(0) ?? "—"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
