import { useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import { Modal } from "../ui/modal";
import { getEvidenceObjectUrl } from "../../services/cases";

interface EvidencePreviewProps {
  caseId: string;
  evidenceId: number;
  filename: string;
  mediaType?: string;
  contentType?: string;
}

export default function EvidencePreview({
  caseId,
  evidenceId,
  filename,
  mediaType,
  contentType,
}: EvidencePreviewProps) {
  const { t } = useTranslation();
  // media_type is normalized ("image"/"pdf") for evidence uploaded through
  // this app, but imported data reuses the column for other taxonomies (e.g.
  // "facial", "video", or a raw MIME string), so content_type — which is
  // consistently a real MIME type — is the source of truth when present.
  const isImage =
    contentType?.startsWith("image/") ?? mediaType === "image";
  const isPdf =
    (contentType ? contentType === "application/pdf" : undefined) ??
    mediaType === "pdf";

  const [url, setUrl] = useState<string | null>(null);
  const [isOpen, setIsOpen] = useState(false);
  const urlRef = useRef<string | null>(null);

  useEffect(() => {
    return () => {
      if (urlRef.current) URL.revokeObjectURL(urlRef.current);
    };
  }, []);

  // Images are fetched eagerly so they can render a thumbnail; PDFs are fetched
  // on demand when the viewer opens.
  useEffect(() => {
    if (!isImage) return;
    let cancelled = false;
    getEvidenceObjectUrl(caseId, evidenceId)
      .then((u) => {
        if (cancelled) {
          URL.revokeObjectURL(u);
          return;
        }
        urlRef.current = u;
        setUrl(u);
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [caseId, evidenceId, isImage]);

  async function open() {
    setIsOpen(true);
    if (urlRef.current) return;
    try {
      const u = await getEvidenceObjectUrl(caseId, evidenceId);
      urlRef.current = u;
      setUrl(u);
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : t("markTraces.loadError"),
      );
    }
  }

  if (!isImage && !isPdf) {
    return <span className="text-gray-400 dark:text-gray-500">—</span>;
  }

  return (
    <>
      {isImage ? (
        <button
          type="button"
          onClick={open}
          className="block h-12 w-12 overflow-hidden rounded-lg border border-gray-200 dark:border-gray-700"
        >
          {url ? (
            <img
              src={url}
              alt={filename}
              className="h-full w-full object-cover"
            />
          ) : (
            <div className="h-full w-full bg-gray-100 dark:bg-gray-800" />
          )}
        </button>
      ) : (
        <button
          type="button"
          onClick={open}
          className="inline-flex h-9 items-center gap-1.5 rounded-lg border border-gray-200 bg-white px-3 text-xs font-medium text-gray-700 shadow-theme-xs hover:bg-gray-50 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-300 dark:hover:bg-white/[0.03]"
        >
          <svg
            className="size-4 text-error-500"
            width="16"
            height="16"
            viewBox="0 0 16 16"
            fill="currentColor"
          >
            <path d="M3 1.5h6.5L13 5v9.5H3V1.5zM9 2v3h3L9 2zM5 8h6v1H5V8zm0 2h6v1H5v-1z" />
          </svg>
          PDF
        </button>
      )}

      <Modal
        isOpen={isOpen}
        onClose={() => setIsOpen(false)}
        className="max-w-4xl m-4"
      >
        <div className="relative p-4">
          {isImage ? (
            url && (
              <img
                src={url}
                alt={filename}
                className="max-h-[80vh] w-full rounded-xl object-contain"
              />
            )
          ) : url ? (
            <iframe
              src={url}
              title={filename}
              className="h-[80vh] w-full rounded-xl border-0"
            />
          ) : (
            <p className="py-16 text-center text-sm text-gray-500 dark:text-gray-400">
              Loading…
            </p>
          )}
          <p className="mt-3 text-center text-sm text-gray-500 dark:text-gray-400">
            {filename}
          </p>
        </div>
      </Modal>
    </>
  );
}
