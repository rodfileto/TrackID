import { useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { Modal } from "../ui/modal";
import Button from "../ui/button/Button";
import {
  ImageViewer,
  type ImageViewerHandle,
} from "../ui/image-viewer/ImageViewer";
import {
  addPoints,
  deletePoint,
  getEvidenceObjectUrl,
  listPoints,
  saveCodificationImage,
  updatePoint,
  type Point,
} from "../../services/cases";

interface Box {
  x1: number;
  y1: number;
  x2: number;
  y2: number;
}

interface CodificationEditorModalProps {
  caseId: string;
  /** FINGERPRINT traces get the full points (minutiae) CRUD; any other case
   * type (FACIAL) only gets the image viewer with brightness/contrast/
   * saturation adjustment -- a face's codification is a computed embedding,
   * not something marked point by point, but the same image inspection
   * tooling is still useful for a closer look. */
  caseType: string;
  evidenceId: number;
  traceId: number;
  traceLabel: string;
  /** The trace's bounding box (case_traces.box_x1..y2), in the evidence
   * image's pixel coordinates. The editor crops to exactly this region --
   * editing a trace's codification only ever concerns that trace, not the
   * rest of the evidence photo. */
  box: Box;
  isOpen: boolean;
  onClose: () => void;
}

/** Crops srcUrl down to box (in the source image's own pixel coordinates)
 * and returns an object URL for the result. The caller owns both URLs and
 * is responsible for revoking them. */
async function cropImage(srcUrl: string, box: Box): Promise<string> {
  const img = new window.Image();
  img.crossOrigin = "anonymous";
  img.src = srcUrl;
  await new Promise<void>((resolve, reject) => {
    img.onload = () => resolve();
    img.onerror = () => reject(new Error("Could not load image to crop"));
  });

  const width = Math.max(1, Math.round(box.x2 - box.x1));
  const height = Math.max(1, Math.round(box.y2 - box.y1));
  const canvas = document.createElement("canvas");
  canvas.width = width;
  canvas.height = height;
  const ctx = canvas.getContext("2d");
  if (!ctx) throw new Error("Could not crop image");
  ctx.drawImage(img, box.x1, box.y1, width, height, 0, 0, width, height);

  return new Promise<string>((resolve, reject) => {
    canvas.toBlob((blob) => {
      if (!blob) {
        reject(new Error("Could not crop image"));
        return;
      }
      resolve(URL.createObjectURL(blob));
    }, "image/png");
  });
}

interface PointDraft {
  x: string;
  y: string;
  pointType: string;
  angle: string;
}

const emptyDraft: PointDraft = { x: "", y: "", pointType: "", angle: "" };

const inputClass =
  "rounded-lg border border-gray-300 bg-transparent px-2 py-1 text-sm text-gray-800 shadow-theme-xs focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90 dark:focus:border-brand-800";

// Point.x/y come from the API in full-evidence-image coordinates, but the
// viewer here shows a crop of just the trace's box -- so drafts (what the
// user sees and edits) are box-relative, converted back to absolute
// coordinates only at the API boundary (handleAdd/handleSave).

function toDraft(point: Point, box: Box): PointDraft {
  return {
    x: String(point.x - box.x1),
    y: String(point.y - box.y1),
    pointType: point.pointType ?? "",
    angle: point.angle != null && point.angle !== 0 ? String(point.angle) : "",
  };
}

/** Parses a draft's text inputs (box-relative) into an absolute PointInput,
 * or null if x/y (or a non-empty angle) aren't valid numbers. */
function parseDraft(
  draft: PointDraft,
  box: Box,
): { x: number; y: number; pointType?: string; angle?: number } | null {
  const x = Number(draft.x);
  const y = Number(draft.y);
  if (!Number.isFinite(x) || !Number.isFinite(y)) return null;
  let angle: number | undefined;
  if (draft.angle.trim() !== "") {
    angle = Number(draft.angle);
    if (!Number.isFinite(angle)) return null;
  }
  return {
    x: x + box.x1,
    y: y + box.y1,
    pointType: draft.pointType.trim() || undefined,
    angle,
  };
}

export default function CodificationEditorModal({
  caseId,
  caseType,
  evidenceId,
  traceId,
  traceLabel,
  box,
  isOpen,
  onClose,
}: CodificationEditorModalProps) {
  const supportsPoints = caseType === "FINGERPRINT";
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [imageUrl, setImageUrl] = useState<string | null>(null);
  const [points, setPoints] = useState<Point[]>([]);
  const [drafts, setDrafts] = useState<Record<number, PointDraft>>({});
  const [savingId, setSavingId] = useState<number | null>(null);
  const [deletingId, setDeletingId] = useState<number | null>(null);
  const [newDraft, setNewDraft] = useState<PointDraft>(emptyDraft);
  const [adding, setAdding] = useState(false);
  const [savingImage, setSavingImage] = useState(false);
  const imageViewerRef = useRef<ImageViewerHandle>(null);

  useEffect(() => {
    if (!isOpen) return;
    let cancelled = false;

    setLoading(true);
    setError("");

    Promise.all([
      getEvidenceObjectUrl(caseId, evidenceId),
      supportsPoints ? listPoints(caseId, evidenceId, traceId) : Promise.resolve([]),
    ])
      .then(async ([fullUrl, existing]) => {
        if (cancelled) {
          URL.revokeObjectURL(fullUrl);
          return;
        }
        const croppedUrl = await cropImage(fullUrl, box);
        URL.revokeObjectURL(fullUrl);
        if (cancelled) {
          URL.revokeObjectURL(croppedUrl);
          return;
        }
        setImageUrl(croppedUrl);
        setPoints(existing);
        setDrafts(
          Object.fromEntries(existing.map((p) => [p.id, toDraft(p, box)])),
        );
      })
      .catch((err) => {
        if (!cancelled) {
          setError(
            err instanceof Error ? err.message : "Could not load codification",
          );
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
    // Depend on box's fields, not the object -- the parent may pass a new
    // object with the same values on every render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    isOpen,
    caseId,
    evidenceId,
    traceId,
    supportsPoints,
    box.x1,
    box.y1,
    box.x2,
    box.y2,
  ]);

  useEffect(() => {
    return () => {
      if (imageUrl) URL.revokeObjectURL(imageUrl);
    };
  }, [imageUrl]);

  function updateDraft(pointId: number, patch: Partial<PointDraft>) {
    setDrafts((current) => ({
      ...current,
      [pointId]: { ...(current[pointId] ?? emptyDraft), ...patch },
    }));
  }

  async function handleAdd() {
    const parsed = parseDraft(newDraft, box);
    if (!parsed) {
      toast.error("X and Y must be numbers.");
      return;
    }
    setAdding(true);
    try {
      const created = await addPoints(caseId, evidenceId, traceId, [parsed]);
      setPoints((current) => [...current, ...created]);
      setDrafts((current) => ({
        ...current,
        ...Object.fromEntries(created.map((p) => [p.id, toDraft(p, box)])),
      }));
      setNewDraft(emptyDraft);
      toast.success("Point added.");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Could not add point");
    } finally {
      setAdding(false);
    }
  }

  async function handleSave(point: Point) {
    const draft = drafts[point.id];
    const parsed = draft && parseDraft(draft, box);
    if (!parsed) {
      toast.error("X and Y must be numbers.");
      return;
    }
    setSavingId(point.id);
    try {
      await updatePoint(caseId, evidenceId, traceId, point.id, parsed);
      setPoints((current) =>
        current.map((p) => (p.id === point.id ? { ...p, ...parsed } : p)),
      );
      toast.success("Point saved.");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Could not save point");
    } finally {
      setSavingId(null);
    }
  }

  async function handleDelete(point: Point) {
    if (
      !window.confirm(`Delete point #${point.sequence}? This cannot be undone.`)
    ) {
      return;
    }
    setDeletingId(point.id);
    try {
      await deletePoint(caseId, evidenceId, traceId, point.id);
      setPoints((current) => current.filter((p) => p.id !== point.id));
      setDrafts((current) => {
        const next = { ...current };
        delete next[point.id];
        return next;
      });
      toast.success("Point deleted.");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Could not delete point");
    } finally {
      setDeletingId(null);
    }
  }

  async function handleSaveImage() {
    const dataUrl = imageViewerRef.current?.exportDataURL();
    if (!dataUrl) {
      toast.error("Image isn't ready yet.");
      return;
    }
    setSavingImage(true);
    try {
      const response = await fetch(dataUrl);
      const blob = await response.blob();
      await saveCodificationImage(caseId, evidenceId, traceId, blob);
      toast.success("Codification image saved.");
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : "Could not save codification image",
      );
    } finally {
      setSavingImage(false);
    }
  }

  return (
    <Modal isOpen={isOpen} onClose={onClose} className="m-4 max-w-6xl">
      <div className="p-4">
        <h4 className="mb-1 text-lg font-medium text-gray-800 dark:text-white/90">
          Edit codification
        </h4>
        <p className="mb-4 text-sm text-gray-500 dark:text-gray-400">
          {traceLabel}
          {!supportsPoints && " — image adjustment only"}
        </p>

        {loading && (
          <p className="py-16 text-center text-sm text-gray-500 dark:text-gray-400">
            Loading…
          </p>
        )}

        {!loading && error && (
          <p className="py-16 text-center text-sm text-error-500">{error}</p>
        )}

        {!loading && !error && imageUrl && !supportsPoints && (
          <ImageViewer
            ref={imageViewerRef}
            src={imageUrl}
            showFilters
            className="h-[70vh] w-full rounded-xl"
          />
        )}

        {!loading && !error && imageUrl && supportsPoints && (
          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
            <ImageViewer
              ref={imageViewerRef}
              src={imageUrl}
              showFilters
              className="h-[60vh] w-full rounded-xl"
            />

            <div className="flex max-h-[60vh] flex-col overflow-y-auto">
              <table className="w-full text-left text-sm">
                <thead>
                  <tr className="text-xs text-gray-500 dark:text-gray-400">
                    <th className="pb-2 pr-2 font-medium">#</th>
                    <th className="pb-2 pr-2 font-medium">X</th>
                    <th className="pb-2 pr-2 font-medium">Y</th>
                    <th className="pb-2 pr-2 font-medium">Type</th>
                    <th className="pb-2 pr-2 font-medium">Angle</th>
                    <th className="pb-2" />
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-100 dark:divide-white/[0.05]">
                  {points.map((point) => {
                    const draft = drafts[point.id] ?? toDraft(point, box);
                    return (
                      <tr key={point.id}>
                        <td className="py-1.5 pr-2 text-gray-500 dark:text-gray-400">
                          {point.sequence}
                        </td>
                        <td className="py-1.5 pr-2">
                          <input
                            type="number"
                            value={draft.x}
                            onChange={(e) =>
                              updateDraft(point.id, { x: e.target.value })
                            }
                            className={`w-16 ${inputClass}`}
                          />
                        </td>
                        <td className="py-1.5 pr-2">
                          <input
                            type="number"
                            value={draft.y}
                            onChange={(e) =>
                              updateDraft(point.id, { y: e.target.value })
                            }
                            className={`w-16 ${inputClass}`}
                          />
                        </td>
                        <td className="py-1.5 pr-2">
                          <input
                            type="text"
                            value={draft.pointType}
                            onChange={(e) =>
                              updateDraft(point.id, { pointType: e.target.value })
                            }
                            placeholder="ENDING…"
                            className={`w-24 ${inputClass}`}
                          />
                        </td>
                        <td className="py-1.5 pr-2">
                          <input
                            type="number"
                            value={draft.angle}
                            onChange={(e) =>
                              updateDraft(point.id, { angle: e.target.value })
                            }
                            className={`w-16 ${inputClass}`}
                          />
                        </td>
                        <td className="whitespace-nowrap py-1.5">
                          <button
                            type="button"
                            onClick={() => handleSave(point)}
                            disabled={savingId === point.id}
                            className="mr-2 text-xs text-brand-500 hover:text-brand-600 disabled:opacity-50"
                          >
                            {savingId === point.id ? "Saving..." : "Save"}
                          </button>
                          <button
                            type="button"
                            onClick={() => handleDelete(point)}
                            disabled={deletingId === point.id}
                            className="text-xs text-error-500 hover:text-error-600 disabled:opacity-50 dark:text-error-400"
                          >
                            {deletingId === point.id ? "Deleting..." : "Delete"}
                          </button>
                        </td>
                      </tr>
                    );
                  })}
                  {points.length === 0 && (
                    <tr>
                      <td
                        colSpan={6}
                        className="py-6 text-center text-gray-500 dark:text-gray-400"
                      >
                        No points yet.
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>

              <div className="mt-4 flex flex-wrap items-end gap-2 border-t border-gray-100 pt-4 dark:border-white/[0.05]">
                <div>
                  <label className="mb-1 block text-xs text-gray-500 dark:text-gray-400">
                    X
                  </label>
                  <input
                    type="number"
                    value={newDraft.x}
                    onChange={(e) =>
                      setNewDraft((d) => ({ ...d, x: e.target.value }))
                    }
                    className={`w-16 ${inputClass}`}
                  />
                </div>
                <div>
                  <label className="mb-1 block text-xs text-gray-500 dark:text-gray-400">
                    Y
                  </label>
                  <input
                    type="number"
                    value={newDraft.y}
                    onChange={(e) =>
                      setNewDraft((d) => ({ ...d, y: e.target.value }))
                    }
                    className={`w-16 ${inputClass}`}
                  />
                </div>
                <div>
                  <label className="mb-1 block text-xs text-gray-500 dark:text-gray-400">
                    Type
                  </label>
                  <input
                    type="text"
                    value={newDraft.pointType}
                    onChange={(e) =>
                      setNewDraft((d) => ({ ...d, pointType: e.target.value }))
                    }
                    placeholder="ENDING…"
                    className={`w-24 ${inputClass}`}
                  />
                </div>
                <div>
                  <label className="mb-1 block text-xs text-gray-500 dark:text-gray-400">
                    Angle
                  </label>
                  <input
                    type="number"
                    value={newDraft.angle}
                    onChange={(e) =>
                      setNewDraft((d) => ({ ...d, angle: e.target.value }))
                    }
                    className={`w-16 ${inputClass}`}
                  />
                </div>
                <Button size="sm" onClick={handleAdd} disabled={adding}>
                  {adding ? "Adding..." : "Add point"}
                </Button>
              </div>
            </div>
          </div>
        )}

        <div className="mt-4 flex justify-end gap-2">
          <Button size="sm" variant="outline" onClick={onClose}>
            Close
          </Button>
          <Button
            size="sm"
            onClick={handleSaveImage}
            disabled={!imageUrl || savingImage}
          >
            {savingImage ? "Saving..." : "Save image"}
          </Button>
        </div>
      </div>
    </Modal>
  );
}
