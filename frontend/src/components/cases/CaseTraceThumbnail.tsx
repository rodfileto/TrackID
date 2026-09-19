import { useEffect, useRef, useState } from "react";
import { Modal } from "../ui/modal";
import { getEvidenceObjectUrl } from "../../services/cases";
import { cropImage } from "../../utils/cropImage";
import type { ThumbnailBox } from "../../services/persons";

interface CaseTraceThumbnailProps {
  caseId: string;
  thumbnailFileId?: number;
  thumbnailBox?: ThumbnailBox;
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

/** A matching case trace's face, cropped out of its evidence image (or
 * shown whole when the trace already has its own dedicated face_crop file --
 * see CaseFaceSearchResult) and rendered the same way IdentityFileThumbnail
 * renders an enrollment photo: a 48x48 thumbnail that opens a full-size
 * viewer on click. */
export default function CaseTraceThumbnail({
  caseId,
  thumbnailFileId,
  thumbnailBox,
  alt,
}: CaseTraceThumbnailProps) {
  const [url, setUrl] = useState<string | null>(null);
  const [failed, setFailed] = useState(false);
  const [isOpen, setIsOpen] = useState(false);
  const ownedUrls = useRef<string[]>([]);

  useEffect(() => {
    return () => {
      for (const owned of ownedUrls.current) URL.revokeObjectURL(owned);
    };
  }, []);

  useEffect(() => {
    if (!thumbnailFileId) return;
    let cancelled = false;

    (async () => {
      try {
        const evidenceUrl = await getEvidenceObjectUrl(caseId, thumbnailFileId);
        if (cancelled) {
          URL.revokeObjectURL(evidenceUrl);
          return;
        }
        ownedUrls.current.push(evidenceUrl);

        if (!thumbnailBox) {
          setUrl(evidenceUrl);
          return;
        }
        const croppedUrl = await cropImage(evidenceUrl, thumbnailBox);
        if (cancelled) {
          URL.revokeObjectURL(croppedUrl);
          return;
        }
        ownedUrls.current.push(croppedUrl);
        setUrl(croppedUrl);
      } catch {
        if (!cancelled) setFailed(true);
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [caseId, thumbnailFileId, thumbnailBox]);

  if (!thumbnailFileId || failed) {
    return <PlaceholderIcon />;
  }

  return (
    <>
      <button
        type="button"
        onClick={() => setIsOpen(true)}
        disabled={!url}
        className="block h-12 w-12 overflow-hidden rounded-lg border border-gray-200 dark:border-gray-700"
      >
        {url ? (
          <img src={url} alt={alt} className="h-full w-full object-cover" />
        ) : (
          <div className="h-full w-full bg-gray-100 dark:bg-gray-800" />
        )}
      </button>

      <Modal
        isOpen={isOpen}
        onClose={() => setIsOpen(false)}
        className="max-w-4xl m-4"
      >
        <div className="relative p-4">
          {url && (
            <img
              src={url}
              alt={alt}
              className="max-h-[80vh] w-full rounded-xl object-contain"
            />
          )}
          <p className="mt-3 text-center text-sm text-gray-500 dark:text-gray-400">
            {alt}
          </p>
        </div>
      </Modal>
    </>
  );
}
