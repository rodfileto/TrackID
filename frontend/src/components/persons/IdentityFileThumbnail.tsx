import { useEffect, useRef, useState } from "react";
import { ImageViewer } from "../ui/image-viewer/ImageViewer";
import { Modal } from "../ui/modal";
import { getIdentityFileObjectUrl } from "../../services/persons";

interface IdentityFileThumbnailProps {
  personId: string;
  identityFileId: number;
  contentType?: string;
  alt: string;
}

function PlaceholderIcon() {
  return (
    <div className="flex h-12 w-12 items-center justify-center rounded-lg border border-gray-200 bg-gray-50 text-gray-300 dark:border-gray-700 dark:bg-white/[0.03] dark:text-gray-600">
      <svg
        width="24"
        height="24"
        viewBox="0 0 24 24"
        fill="none"
        xmlns="http://www.w3.org/2000/svg"
      >
        <path
          d="M4 6a2 2 0 0 1 2-2h12a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6Z"
          stroke="currentColor"
          strokeWidth="1.5"
        />
        <path
          d="m4 16 4.5-4.5a1.5 1.5 0 0 1 2.12 0L14 14.9M14.5 12.5l1.38-1.38a1.5 1.5 0 0 1 2.12 0L20 13"
          stroke="currentColor"
          strokeWidth="1.5"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
        <circle cx="9" cy="8.5" r="1.5" stroke="currentColor" strokeWidth="1.5" />
      </svg>
    </div>
  );
}

function PdfIcon() {
  return (
    <svg
      className="size-4 text-error-500"
      width="16"
      height="16"
      viewBox="0 0 16 16"
      fill="currentColor"
    >
      <path d="M3 1.5h6.5L13 5v9.5H3V1.5zM9 2v3h3L9 2zM5 8h6v1H5V8zm0 2h6v1H5v-1z" />
    </svg>
  );
}

export default function IdentityFileThumbnail({
  personId,
  identityFileId,
  contentType,
  alt,
}: IdentityFileThumbnailProps) {
  // content_type is consistently a real MIME type when present (see
  // EvidencePreview's identical check), so it's the source of truth for
  // whether this file is a displayable image or PDF.
  const isImage = contentType?.startsWith("image/") ?? false;
  const isPdf = contentType === "application/pdf";

  const [url, setUrl] = useState<string | null>(null);
  const [failed, setFailed] = useState(false);
  const [isOpen, setIsOpen] = useState(false);
  const urlRef = useRef<string | null>(null);

  useEffect(() => {
    return () => {
      if (urlRef.current) URL.revokeObjectURL(urlRef.current);
    };
  }, []);

  // Images are fetched eagerly so a thumbnail can render; PDFs are fetched
  // on demand when the viewer opens (see open() below).
  useEffect(() => {
    if (!isImage) return;
    let cancelled = false;
    getIdentityFileObjectUrl(personId, identityFileId)
      .then((u) => {
        if (cancelled) {
          URL.revokeObjectURL(u);
          return;
        }
        urlRef.current = u;
        setUrl(u);
      })
      .catch(() => {
        if (!cancelled) setFailed(true);
      });
    return () => {
      cancelled = true;
    };
  }, [personId, identityFileId, isImage]);

  async function open() {
    setIsOpen(true);
    if (urlRef.current) return;
    try {
      const u = await getIdentityFileObjectUrl(personId, identityFileId);
      urlRef.current = u;
      setUrl(u);
    } catch {
      setFailed(true);
    }
  }

  if (!isImage && !isPdf) {
    return <PlaceholderIcon />;
  }

  return (
    <>
      {isImage ? (
        failed ? (
          <PlaceholderIcon />
        ) : (
          <button
            type="button"
            onClick={open}
            className="block h-12 w-12 overflow-hidden rounded-lg border border-gray-200 dark:border-gray-700"
          >
            {url ? (
              <ImageViewer
                src={url}
                showToolbar={false}
                showZoomIndicator={false}
                showRotate={false}
                draggable={false}
                className="h-12 w-12"
              />
            ) : (
              <div className="h-full w-full bg-gray-100 dark:bg-gray-800" />
            )}
          </button>
        )
      ) : (
        <button
          type="button"
          onClick={open}
          className="inline-flex h-9 items-center gap-1.5 rounded-lg border border-gray-200 bg-white px-3 text-xs font-medium text-gray-700 shadow-theme-xs hover:bg-gray-50 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-300 dark:hover:bg-white/[0.03]"
        >
          <PdfIcon />
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
              <ImageViewer src={url} className="h-[70vh] w-full rounded-xl" />
            )
          ) : url ? (
            <iframe
              src={url}
              title={alt}
              className="h-[80vh] w-full rounded-xl border-0"
            />
          ) : (
            <p className="py-16 text-center text-sm text-gray-500 dark:text-gray-400">
              Loading…
            </p>
          )}
          <p className="mt-3 text-center text-sm text-gray-500 dark:text-gray-400">
            {alt}
          </p>
        </div>
      </Modal>
    </>
  );
}
