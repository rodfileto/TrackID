import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Modal } from "../ui/modal";
import { ImageViewer } from "../ui/image-viewer/ImageViewer";
import { getPairImageUrl } from "../../services/analyses";
import type { TraceLink } from "../../services/cases";

/** One side of a pair, large enough to compare faces: a face of a case (cut to its box by the
 * server) or an enrolled person's photo, loaded through the analysis by feature id, so a blind
 * examiner's page needs nothing that says whose face it is. Clicking opens a zoomable viewer with
 * brightness and contrast controls. */
export default function FaceImage({
  caseId,
  analysisId,
  face,
  label,
}: {
  caseId: string;
  analysisId: number;
  face: TraceLink;
  label: string;
}) {
  const { t } = useTranslation();
  const [url, setUrl] = useState<string | null>(null);
  const [failed, setFailed] = useState(false);
  const [open, setOpen] = useState(false);

  useEffect(() => {
    let cancelled = false;
    const owned: string[] = [];
    (async () => {
      try {
        if (!face.featureId) throw new Error("no image");
        const src = await getPairImageUrl(caseId, analysisId, face.featureId);
        owned.push(src);
        if (!cancelled) setUrl(src);
      } catch {
        if (!cancelled) setFailed(true);
      }
    })();
    return () => {
      cancelled = true;
      for (const u of owned) URL.revokeObjectURL(u);
    };
  }, [caseId, analysisId, face]);

  return (
    <figure className="flex flex-col gap-2">
      <button
        type="button"
        onClick={() => setOpen(true)}
        disabled={!url}
        className="flex h-64 w-full items-center justify-center overflow-hidden rounded-xl border border-gray-200 bg-gray-50 dark:border-gray-800 dark:bg-white/[0.03]"
      >
        {url ? (
          <img src={url} alt={label} className="h-full w-full object-contain" />
        ) : (
          <span className="text-xs text-gray-400">{failed ? t("analyses.noImage") : t("common.loading")}</span>
        )}
      </button>
      <figcaption className="text-xs text-gray-500 dark:text-gray-400">{label}</figcaption>
      <Modal isOpen={open} onClose={() => setOpen(false)} className="max-w-5xl m-4">
        <div className="p-4">
          {url && <ImageViewer src={url} className="h-[75vh]" showToolbar showZoomIndicator showFilters draggable />}
          <p className="mt-3 text-center text-sm text-gray-500 dark:text-gray-400">{label}</p>
        </div>
      </Modal>
    </figure>
  );
}
