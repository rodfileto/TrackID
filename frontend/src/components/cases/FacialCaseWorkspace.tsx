import { useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import ComponentCard from "../common/ComponentCard";
import Button from "../ui/button/Button";
import {
  ImageViewer,
  type ImageViewerHandle,
} from "../ui/image-viewer/ImageViewer";
import EvidenceDropzone from "./EvidenceDropzone";
import TraceEditor, { type TraceBox } from "./TraceEditor";
import CaseClusters from "./CaseClusters";
import {
  addEvidence,
  compareFaces,
  CompareFacesError,
  createTraces,
  deleteEvidence,
  deleteTrace,
  detectFaces,
  getEvidenceObjectUrl,
  listCaseClusters,
  listTraces,
  saveCodificationImage,
  type CaseCluster,
  type EmbeddingSource,
  type Evidence,
  type FaceComparison,
  type FaceProposal,
  type Trace,
} from "../../services/cases";
import {
  codificationLabel,
  useCodificationThumbnails,
  type CodificationThumbnail,
} from "../../hooks/useCodificationThumbnails";

interface FacialCaseWorkspaceProps {
  caseId: string;
  evidences: Evidence[];
  /** Reload the case detail after evidence is added or excluded. */
  onEvidencesChanged: () => Promise<void>;
}

/** Slot index into the comparison region: 0 = left, 1 = right. */
type SlotIds = [number | null, number | null];

/** A detected box counts as already marked when it overlaps an existing box
 * this much -- re-running detection on a reviewed photo proposes nothing new. */
const DUPLICATE_IOU = 0.5;

function isImageEvidence(evidence: Evidence): boolean {
  return evidence.contentType?.startsWith("image/") ?? evidence.mediaType === "image";
}

function toLockedBox(trace: Trace): TraceBox {
  return {
    id: String(trace.id),
    boxX1: trace.boxX1,
    boxY1: trace.boxY1,
    boxX2: trace.boxX2,
    boxY2: trace.boxY2,
    score: trace.score || undefined,
    locked: true,
  };
}

function newBoxId(): string {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return crypto.randomUUID();
  }
  return `proposal-${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

type Box = Pick<TraceBox, "boxX1" | "boxY1" | "boxX2" | "boxY2">;

function iou(a: Box, b: Box): number {
  const width = Math.min(a.boxX2, b.boxX2) - Math.max(a.boxX1, b.boxX1);
  const height = Math.min(a.boxY2, b.boxY2) - Math.max(a.boxY1, b.boxY1);
  if (width <= 0 || height <= 0) return 0;
  const intersection = width * height;
  const area = (box: Box) => (box.boxX2 - box.boxX1) * (box.boxY2 - box.boxY1);
  return intersection / (area(a) + area(b) - intersection);
}

function proposalToBox(proposal: FaceProposal): TraceBox {
  return {
    id: newBoxId(),
    boxX1: proposal.boxX1,
    boxY1: proposal.boxY1,
    boxX2: proposal.boxX2,
    boxY2: proposal.boxY2,
    score: proposal.score,
  };
}

// ─────────────────────────────────────────────────────────────────────────────
// Evidence rail thumbnail
// ─────────────────────────────────────────────────────────────────────────────

function EvidenceThumb({
  caseId,
  evidence,
  selected,
  faceCount,
  pendingCount,
  detecting,
  onSelect,
}: {
  caseId: string;
  evidence: Evidence;
  selected: boolean;
  faceCount: number;
  pendingCount: number;
  detecting: boolean;
  onSelect: () => void;
}) {
  const { t } = useTranslation();
  const [url, setUrl] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    let created: string | null = null;
    getEvidenceObjectUrl(caseId, evidence.id)
      .then((objectUrl) => {
        if (cancelled) {
          URL.revokeObjectURL(objectUrl);
          return;
        }
        created = objectUrl;
        setUrl(objectUrl);
      })
      .catch(() => {});
    return () => {
      cancelled = true;
      if (created) URL.revokeObjectURL(created);
    };
  }, [caseId, evidence.id]);

  return (
    <button
      type="button"
      onClick={onSelect}
      title={evidence.filename}
      className={`flex w-full items-center gap-3 rounded-lg border p-2 text-left transition-colors ${
        selected
          ? "border-brand-500 bg-brand-50 dark:border-brand-500 dark:bg-brand-500/10"
          : "border-gray-200 hover:border-brand-300 dark:border-white/[0.05] dark:hover:border-brand-800"
      }`}
    >
      <span className="h-12 w-12 shrink-0 overflow-hidden rounded-md bg-gray-100 dark:bg-white/[0.05]">
        {url && (
          <img
            src={url}
            alt={evidence.filename}
            className="h-full w-full object-cover"
          />
        )}
      </span>
      <span className="min-w-0 flex-1">
        <span className="block truncate text-xs font-medium text-gray-700 dark:text-gray-300">
          {evidence.filename}
        </span>
        <span className="block text-xs text-gray-400 dark:text-gray-500">
          {t("facial.faceCount", { count: faceCount })}
        </span>
        {detecting ? (
          <span className="block text-xs text-brand-500">
            {t("facial.detecting")}
          </span>
        ) : (
          pendingCount > 0 && (
            <span className="block text-xs font-medium text-warning-600 dark:text-warning-500">
              {t("facial.toReview", { count: pendingCount })}
            </span>
          )
        )}
      </span>
    </button>
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// Machine score
// ─────────────────────────────────────────────────────────────────────────────

const EMBEDDING_SOURCE_LABEL_KEYS: Record<EmbeddingSource, string> = {
  stored: "facial.source.stored",
  codification_image: "facial.source.codificationImage",
  evidence: "facial.source.evidence",
};

/** Order-insensitive: the score is symmetric, so a swap keeps it valid. */
function pairKey(left: number, right: number): string {
  return [left, right].sort((a, b) => a - b).join("-");
}

function MachineScore({
  result,
  slotIds,
}: {
  result: FaceComparison;
  slotIds: SlotIds;
}) {
  const { t } = useTranslation();
  const similarity = Math.max(-1, Math.min(1, result.similarity));
  const sourceLabel = (codificationId: number | null) => {
    const face = result.faces.find((f) => f.codificationId === codificationId);
    return face ? t(EMBEDDING_SOURCE_LABEL_KEYS[face.source]) : "—";
  };

  return (
    <div className="rounded-xl border border-gray-200 px-4 py-3 dark:border-white/[0.05]">
      <div className="flex items-baseline justify-between gap-4">
        <span className="text-sm font-medium text-gray-700 dark:text-gray-300">
          {t("facial.machineScore")}
        </span>
        <span className="font-mono text-2xl font-semibold text-gray-800 dark:text-white/90">
          {similarity.toFixed(3)}
        </span>
      </div>
      <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-gray-100 dark:bg-white/[0.06]">
        <div
          className="h-full rounded-full bg-brand-500"
          style={{ width: `${Math.max(0, similarity) * 100}%` }}
        />
      </div>
      <p className="mt-2 text-xs text-gray-500 dark:text-gray-400">
        {t("facial.cosineHint", { model: result.modelVersion })}
      </p>
      <p className="mt-1 text-xs text-gray-400 dark:text-gray-500">
        A: {sourceLabel(slotIds[0])} · B: {sourceLabel(slotIds[1])}
      </p>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// Comparison pane
// ─────────────────────────────────────────────────────────────────────────────

function ComparePane({
  caseId,
  slot,
  face,
  onClear,
}: {
  caseId: string;
  slot: string;
  face: CodificationThumbnail | null;
  onClear: () => void;
}) {
  const { t } = useTranslation();
  const viewerRef = useRef<ImageViewerHandle>(null);
  const [saving, setSaving] = useState(false);

  async function handleSaveImage() {
    if (!face) return;
    const dataUrl = viewerRef.current?.exportDataURL();
    if (!dataUrl) {
      toast.error(t("codification.imageNotReady"));
      return;
    }
    setSaving(true);
    try {
      const response = await fetch(dataUrl);
      const blob = await response.blob();
      await saveCodificationImage(
        caseId,
        face.codification.evidenceFileId,
        face.codification.traceId,
        blob,
      );
      toast.success(
        t("facial.imageSavedFor", { label: codificationLabel(face.codification) }),
      );
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : t("codification.imageSaveError"),
      );
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="flex flex-col overflow-hidden rounded-xl border border-gray-200 dark:border-white/[0.05]">
      <div className="flex items-center justify-between gap-2 border-b border-gray-100 px-3 py-2 dark:border-white/[0.05]">
        <div className="flex min-w-0 items-center gap-2">
          <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded bg-gray-100 text-xs font-medium text-gray-600 dark:bg-white/[0.06] dark:text-gray-300">
            {slot}
          </span>
          {face ? (
            <span className="min-w-0">
              <span className="block truncate text-sm font-medium text-gray-800 dark:text-white/90">
                {codificationLabel(face.codification)}
              </span>
              <span className="block truncate text-xs text-gray-400 dark:text-gray-500">
                {face.codification.evidenceFilename}
              </span>
            </span>
          ) : (
            <span className="text-sm text-gray-400 dark:text-gray-500">
              {t("facial.empty")}
            </span>
          )}
        </div>

        {face && (
          <div className="flex shrink-0 items-center gap-3">
            <button
              type="button"
              onClick={handleSaveImage}
              disabled={saving}
              className="text-xs text-brand-500 hover:text-brand-600 disabled:opacity-50"
            >
              {saving ? t("common.saving") : t("codification.saveImage")}
            </button>
            <button
              type="button"
              onClick={onClear}
              className="text-xs text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
            >
              {t("common.clear")}
            </button>
          </div>
        )}
      </div>

      {face ? (
        <ImageViewer
          ref={viewerRef}
          src={face.thumbnailUrl}
          showFilters
          className="h-[46vh] w-full"
        />
      ) : (
        <p className="flex h-[46vh] items-center justify-center px-4 text-center text-sm text-gray-400 dark:text-gray-500">
          {t("facial.pickFace")}
        </p>
      )}
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// FacialCaseWorkspace
// ─────────────────────────────────────────────────────────────────────────────

export default function FacialCaseWorkspace({
  caseId,
  evidences,
  onEvidencesChanged,
}: FacialCaseWorkspaceProps) {
  const { t } = useTranslation();
  const imageEvidences = evidences.filter(isImageEvidence);

  const [selectedEvidenceId, setSelectedEvidenceId] = useState<number | null>(
    null,
  );
  const selectedEvidenceIdRef = useRef(selectedEvidenceId);
  selectedEvidenceIdRef.current = selectedEvidenceId;

  const [evidenceUrl, setEvidenceUrl] = useState<string | null>(null);
  /** Boxes already saved as traces on the selected evidence (read-only). */
  const [savedBoxes, setSavedBoxes] = useState<TraceBox[]>([]);
  /** Unsaved boxes per evidence file id -- drawn by hand or proposed by face
   * detection. Nothing here is persisted until the analyst clicks Codify. */
  const [pending, setPending] = useState<Record<number, TraceBox[]>>({});
  const [detectingIds, setDetectingIds] = useState<number[]>([]);
  const [editorLoading, setEditorLoading] = useState(false);
  const [savingTraces, setSavingTraces] = useState(false);
  const [drawing, setDrawing] = useState(false);

  const [selectedFiles, setSelectedFiles] = useState<File[]>([]);
  const [uploading, setUploading] = useState(false);

  const [refreshKey, setRefreshKey] = useState(0);
  const { items: faces, loading: facesLoading, error: facesError } =
    useCodificationThumbnails(caseId, refreshKey);

  const [clusters, setClusters] = useState<CaseCluster[]>([]);
  const [clustersLoading, setClustersLoading] = useState(true);
  const [clustersError, setClustersError] = useState("");

  useEffect(() => {
    let cancelled = false;
    setClustersLoading(true);
    setClustersError("");
    listCaseClusters(caseId)
      .then((items) => {
        if (cancelled) return;
        setClusters(items);
      })
      .catch((err) => {
        if (cancelled) return;
        setClustersError(
          err instanceof Error ? err.message : t("facial.clustersLoadError"),
        );
      })
      .finally(() => {
        if (!cancelled) setClustersLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [caseId, refreshKey, t]);

  const [slotIds, setSlotIds] = useState<SlotIds>([null, null]);
  const [comparison, setComparison] = useState<{
    key: string;
    result: FaceComparison;
  } | null>(null);
  const [scoring, setScoring] = useState(false);
  const [deletingTraceId, setDeletingTraceId] = useState<number | null>(null);

  const selectedEvidence =
    imageEvidences.find((item) => item.id === selectedEvidenceId) ?? null;

  // Keep the selection pointing at an evidence file that still exists --
  // falls back to the first one on load and after an exclude.
  useEffect(() => {
    const exists = imageEvidences.some((item) => item.id === selectedEvidenceId);
    if (!exists) setSelectedEvidenceId(imageEvidences[0]?.id ?? null);
  }, [imageEvidences, selectedEvidenceId]);

  // Load the selected evidence image and the traces already saved on it.
  // Codifying or deleting a face updates `savedBoxes` locally instead of
  // reloading here, so the photo keeps its zoom/pan while you work.
  useEffect(() => {
    setDrawing(false);
    if (selectedEvidenceId === null) {
      setEvidenceUrl(null);
      setSavedBoxes([]);
      return;
    }

    let cancelled = false;
    let created: string | null = null;

    setEditorLoading(true);
    Promise.all([
      getEvidenceObjectUrl(caseId, selectedEvidenceId),
      listTraces(caseId, selectedEvidenceId),
    ])
      .then(([url, existing]) => {
        if (cancelled) {
          URL.revokeObjectURL(url);
          return;
        }
        created = url;
        setEvidenceUrl(url);
        setSavedBoxes(existing.map(toLockedBox));
      })
      .catch((err) => {
        if (!cancelled) {
          toast.error(
            err instanceof Error ? err.message : t("markTraces.loadError"),
          );
        }
      })
      .finally(() => {
        if (!cancelled) setEditorLoading(false);
      });

    return () => {
      cancelled = true;
      if (created) URL.revokeObjectURL(created);
    };
  }, [caseId, selectedEvidenceId, t]);

  const unsavedTraces =
    selectedEvidenceId === null ? [] : (pending[selectedEvidenceId] ?? []);
  const traces = [...savedBoxes, ...unsavedTraces];
  const detectingSelected =
    selectedEvidenceId !== null && detectingIds.includes(selectedEvidenceId);

  function setPendingFor(evidenceId: number, boxes: TraceBox[]) {
    setPending((current) => ({ ...current, [evidenceId]: boxes }));
  }

  // The editor hands back the full list; locked (saved) boxes can't change in
  // it, so only the unsaved ones need keeping.
  function handleTracesChange(next: TraceBox[]) {
    if (selectedEvidenceId === null) return;
    setPendingFor(
      selectedEvidenceId,
      next.filter((trace) => !trace.locked),
    );
  }

  /** Runs face detection on one evidence image and queues what it finds as
   * unsaved boxes, skipping any that overlap a box already there. Returns how
   * many were queued, or null if detection failed (an error toast is shown). */
  async function recognizeFaces(
    evidenceId: number,
    savedOnEvidence: Box[],
  ): Promise<number | null> {
    setDetectingIds((ids) => [...ids, evidenceId]);
    try {
      const proposals = await detectFaces(caseId, evidenceId);
      const alreadyThere = (box: Box, others: Box[]) =>
        others.some((other) => iou(box, other) >= DUPLICATE_IOU);

      const fresh = proposals
        .map(proposalToBox)
        .filter(
          (box) =>
            !alreadyThere(box, [...savedOnEvidence, ...(pending[evidenceId] ?? [])]),
        );
      setPending((current) => {
        const queued = current[evidenceId] ?? [];
        return {
          ...current,
          [evidenceId]: [
            ...queued,
            ...fresh.filter((box) => !alreadyThere(box, queued)),
          ],
        };
      });
      return fresh.length;
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : t("facial.detectError"),
      );
      return null;
    } finally {
      setDetectingIds((ids) => ids.filter((id) => id !== evidenceId));
    }
  }

  async function handleRecognizeSelected() {
    if (selectedEvidenceId === null) return;
    setDrawing(false);
    const found = await recognizeFaces(selectedEvidenceId, savedBoxes);
    if (found === null) return;
    if (found === 0) {
      toast.info(t("facial.noNewFaces"));
    } else {
      toast.success(t("facial.detected", { count: found }));
    }
  }

  const slotFaces = slotIds.map((id) =>
    id === null ? null : (faces.find((f) => f.codification.id === id) ?? null),
  );

  function assignToSlot(codificationId: number) {
    setSlotIds(([left, right]) => {
      if (left === codificationId || right === codificationId) {
        return [left, right];
      }
      if (left === null) return [codificationId, right];
      if (right === null) return [left, codificationId];
      // Both taken: keep the most recent pick on the right, shift the other
      // one left, so "compare the next face against this one" keeps working.
      return [right, codificationId];
    });
  }

  const bothSlotsFilled = slotIds[0] !== null && slotIds[1] !== null;
  const currentComparison =
    comparison &&
    slotIds[0] !== null &&
    slotIds[1] !== null &&
    comparison.key === pairKey(slotIds[0], slotIds[1])
      ? comparison.result
      : null;

  async function handleMachineScore() {
    const [left, right] = slotIds;
    if (left === null || right === null) return;
    setScoring(true);
    try {
      const result = await compareFaces(caseId, [left, right]);
      setComparison({ key: pairKey(left, right), result });
    } catch (err) {
      if (err instanceof CompareFacesError && err.codificationId !== undefined) {
        const slot = err.codificationId === left ? "A" : "B";
        const face = slotFaces.find(
          (f) => f?.codification.id === err.codificationId,
        );
        toast.error(
          t("facial.noFaceInSlot", {
            slot,
            label: face ? ` (${codificationLabel(face.codification)})` : "",
          }),
        );
      } else {
        toast.error(
          err instanceof Error ? err.message : t("facial.compareError"),
        );
      }
    } finally {
      setScoring(false);
    }
  }

  function clearSlot(index: 0 | 1) {
    setSlotIds(([left, right]) =>
      index === 0 ? [null, right] : [left, null],
    );
  }

  function swapSlots() {
    setSlotIds(([left, right]) => [right, left]);
  }

  async function handleSaveTraces() {
    const evidenceId = selectedEvidenceId;
    if (evidenceId === null || unsavedTraces.length === 0) return;
    setSavingTraces(true);
    try {
      const created = await createTraces(
        caseId,
        evidenceId,
        unsavedTraces.map((trace) => ({
          boxX1: trace.boxX1,
          boxY1: trace.boxY1,
          boxX2: trace.boxX2,
          boxY2: trace.boxY2,
          score: trace.score,
        })),
      );
      setPendingFor(evidenceId, []);
      // If the analyst switched images mid-save, the next load of this one
      // fetches the new traces anyway.
      if (selectedEvidenceIdRef.current === evidenceId) {
        setSavedBoxes((current) => [...current, ...created.map(toLockedBox)]);
      }
      setRefreshKey((key) => key + 1);
      toast.success(t("facial.codified", { count: created.length }));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("markTraces.saveError"));
    } finally {
      setSavingTraces(false);
    }
  }

  async function handleDeleteFace(face: CodificationThumbnail) {
    const label = codificationLabel(face.codification);
    if (!window.confirm(t("facial.confirmDeleteFace", { label }))) return;

    setDeletingTraceId(face.codification.traceId);
    try {
      await deleteTrace(
        caseId,
        face.codification.evidenceFileId,
        face.codification.traceId,
      );
      setSlotIds(([left, right]) => [
        left === face.codification.id ? null : left,
        right === face.codification.id ? null : right,
      ]);
      setSavedBoxes((current) =>
        current.filter((trace) => trace.id !== String(face.codification.traceId)),
      );
      setRefreshKey((key) => key + 1);
      toast.success(t("facial.faceDeleted", { label }));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("facial.deleteFaceError"));
    } finally {
      setDeletingTraceId(null);
    }
  }

  async function handleUpload() {
    if (selectedFiles.length === 0) return;
    setUploading(true);

    const added: Evidence[] = [];
    let failed = 0;
    for (const file of selectedFiles) {
      try {
        added.push(await addEvidence(caseId, file));
      } catch {
        failed++;
      }
    }

    if (failed === 0) {
      toast.success(t("caseDetail.evidenceAdded", { count: added.length }));
    } else if (added.length > 0) {
      toast.warning(
        t("caseDetail.partialUpload", { added: added.length, failed }),
      );
    } else {
      toast.error(t("caseDetail.noneUploaded"));
    }

    setSelectedFiles([]);
    setUploading(false);
    await onEvidencesChanged();

    // New images get faces detected straight away, as unsaved boxes the
    // analyst still has to review and Codify.
    const images = added.filter(isImageEvidence);
    if (images.length === 0) return;
    setSelectedEvidenceId(images[0].id);

    let found = 0;
    for (const image of images) {
      const count = await recognizeFaces(image.id, []);
      if (count === null) return;
      found += count;
    }
    if (found === 0) {
      toast.info(t("facial.noFacesNew"));
    } else {
      toast.success(t("facial.detected", { count: found }));
    }
  }

  function handleDiscardUnsaved() {
    if (selectedEvidenceId === null) return;
    setDrawing(false);
    setPendingFor(selectedEvidenceId, []);
  }

  async function handleExcludeEvidence(evidence: Evidence) {
    if (
      !window.confirm(
        t("caseDetail.confirmExclude", { filename: evidence.filename }),
      )
    ) {
      return;
    }
    try {
      await deleteEvidence(caseId, evidence.id);
      setPending((current) => {
        const next = { ...current };
        delete next[evidence.id];
        return next;
      });
      toast.success(t("caseDetail.evidenceExcluded"));
      await onEvidencesChanged();
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : t("caseDetail.excludeError"),
      );
    }
  }

  function faceCountFor(evidenceFileId: number): number {
    return faces.filter((f) => f.codification.evidenceFileId === evidenceFileId)
      .length;
  }

  return (
    <div className="space-y-6">
      <ComponentCard
        title={t("facial.evidenceTitle")}
        desc={t("facial.evidenceDesc")}
      >
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-12">
          <div className="flex max-h-[60vh] flex-col gap-2 overflow-y-auto pr-1 lg:col-span-3">
            {imageEvidences.map((evidence) => (
              <EvidenceThumb
                key={evidence.id}
                caseId={caseId}
                evidence={evidence}
                selected={evidence.id === selectedEvidenceId}
                faceCount={faceCountFor(evidence.id)}
                pendingCount={pending[evidence.id]?.length ?? 0}
                detecting={detectingIds.includes(evidence.id)}
                onSelect={() => setSelectedEvidenceId(evidence.id)}
              />
            ))}

            {imageEvidences.length === 0 && (
              <p className="py-4 text-center text-sm text-gray-500 dark:text-gray-400">
                {t("facial.noImageEvidence")}
              </p>
            )}
          </div>

          <div className="lg:col-span-9">
            <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
              <div className="min-w-0">
                <p className="truncate text-sm font-medium text-gray-800 dark:text-white/90">
                  {selectedEvidence?.filename ?? t("facial.noEvidenceSelected")}
                </p>
                {drawing ? (
                  <p className="text-xs text-brand-500">
                    {t("facial.dragToBox")}
                  </p>
                ) : detectingSelected ? (
                  <p className="text-xs text-brand-500">
                    {t("facial.detectingFaces")}
                  </p>
                ) : unsavedTraces.length > 0 ? (
                  <p className="text-xs font-medium text-warning-600 dark:text-warning-500">
                    {t("facial.unsavedBoxes", { count: unsavedTraces.length })}
                  </p>
                ) : (
                  <p className="text-xs text-gray-500 dark:text-gray-400">
                    {t("facial.savedFaces", { count: savedBoxes.length })}
                  </p>
                )}
              </div>
              <div className="flex flex-wrap items-center gap-2">
                {selectedEvidence && (
                  <button
                    type="button"
                    onClick={() => handleExcludeEvidence(selectedEvidence)}
                    className="inline-flex h-9 items-center rounded-lg border border-gray-200 bg-white px-3 text-xs font-medium text-error-500 shadow-theme-xs hover:bg-error-50 dark:border-gray-700 dark:bg-gray-900 dark:text-error-400 dark:hover:bg-white/[0.03]"
                  >
                    {t("facial.excludeEvidence")}
                  </button>
                )}
                <button
                  type="button"
                  onClick={handleRecognizeSelected}
                  disabled={!evidenceUrl || editorLoading || detectingSelected}
                  className="inline-flex h-9 items-center rounded-lg border border-gray-200 bg-white px-3 text-xs font-medium text-gray-700 shadow-theme-xs transition-colors hover:bg-gray-50 disabled:opacity-50 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-300 dark:hover:bg-white/[0.03]"
                >
                  {detectingSelected ? t("facial.detecting") : t("facial.recognize")}
                </button>
                <button
                  type="button"
                  onClick={() => setDrawing((current) => !current)}
                  disabled={!evidenceUrl || editorLoading}
                  className={`inline-flex h-9 items-center rounded-lg border px-3 text-xs font-medium shadow-theme-xs transition-colors disabled:opacity-50 ${
                    drawing
                      ? "border-brand-500 bg-brand-500 text-white"
                      : "border-gray-200 bg-white text-gray-700 hover:bg-gray-50 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-300 dark:hover:bg-white/[0.03]"
                  }`}
                >
                  {drawing ? t("facial.cancelMarking") : t("facial.markFace")}
                </button>
                {unsavedTraces.length > 0 && (
                  <button
                    type="button"
                    onClick={handleDiscardUnsaved}
                    disabled={savingTraces}
                    className="inline-flex h-9 items-center rounded-lg border border-gray-200 bg-white px-3 text-xs font-medium text-gray-700 shadow-theme-xs transition-colors hover:bg-gray-50 disabled:opacity-50 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-300 dark:hover:bg-white/[0.03]"
                  >
                    {t("facial.discard")}
                  </button>
                )}
                <Button
                  size="sm"
                  onClick={handleSaveTraces}
                  disabled={unsavedTraces.length === 0 || savingTraces}
                >
                  {savingTraces
                    ? t("common.saving")
                    : unsavedTraces.length > 0
                      ? t("facial.codifyN", { count: unsavedTraces.length })
                      : t("facial.codify")}
                </Button>
              </div>
            </div>

            <div className="flex flex-col gap-4 md:flex-row">
              <div className="min-w-0 flex-1">
                {editorLoading && (
                  <p className="flex h-[52vh] items-center justify-center rounded-xl border border-gray-200 text-sm text-gray-500 dark:border-white/[0.05] dark:text-gray-400">
                    {t("common.loading")}
                  </p>
                )}

                {!editorLoading && evidenceUrl && (
                  <TraceEditor
                    src={evidenceUrl}
                    traces={traces}
                    onChange={handleTracesChange}
                    drawing={drawing}
                    onDrawingChange={setDrawing}
                    className="h-[52vh] w-full rounded-xl"
                  />
                )}

                {!editorLoading && !evidenceUrl && (
                  <p className="flex h-[52vh] items-center justify-center rounded-xl border border-gray-200 px-4 text-center text-sm text-gray-500 dark:border-white/[0.05] dark:text-gray-400">
                    {t("facial.addImageHint")}
                  </p>
                )}
              </div>

              <div className="flex shrink-0 flex-col gap-3 md:h-[52vh] md:w-48">
                <EvidenceDropzone
                  compact
                  className="md:flex-1"
                  onFiles={(files) =>
                    setSelectedFiles((current) => [...current, ...files])
                  }
                  onRejected={() =>
                    toast.error(t("caseDetail.unsupportedFile"))
                  }
                />
                {selectedFiles.length > 0 && (
                  <div className="flex flex-col gap-2">
                    <span className="text-xs text-gray-500 dark:text-gray-400">
                      {t("facial.filesReady", { count: selectedFiles.length })}
                    </span>
                    <div className="flex items-center justify-between gap-2">
                      <button
                        type="button"
                        onClick={() => setSelectedFiles([])}
                        className="text-xs text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
                      >
                        {t("common.clear")}
                      </button>
                      <Button
                        size="sm"
                        onClick={handleUpload}
                        disabled={uploading}
                      >
                        {uploading ? t("caseDetail.uploading") : t("caseDetail.upload")}
                      </Button>
                    </div>
                  </div>
                )}
              </div>
            </div>
          </div>
        </div>
      </ComponentCard>

      <ComponentCard
        title={t("facial.codifications")}
        desc={
          facesLoading
            ? t("common.loading")
            : t("facial.codificationsDesc", { count: faces.length })
        }
      >
        {facesLoading && (
          <p className="py-8 text-center text-sm text-gray-500 dark:text-gray-400">
            {t("common.loading")}
          </p>
        )}

        {!facesLoading && facesError && (
          <p className="py-8 text-center text-sm text-error-500">{facesError}</p>
        )}

        {!facesLoading && !facesError && faces.length === 0 && (
          <p className="py-8 text-center text-sm text-gray-500 dark:text-gray-400">
            {t("facial.noFacesCodified")}
          </p>
        )}

        {!facesLoading && !facesError && faces.length > 0 && (
          <div className="grid max-h-[46vh] grid-cols-3 gap-3 overflow-y-auto pr-1 sm:grid-cols-4 md:grid-cols-6 lg:grid-cols-8">
            {faces.map((face) => {
              const slotIndex = slotIds.indexOf(face.codification.id);
              return (
                <div
                  key={face.codification.id}
                  className={`group relative overflow-hidden rounded-xl border transition-colors ${
                    slotIndex >= 0
                      ? "border-brand-500"
                      : "border-gray-200 hover:border-brand-300 dark:border-white/[0.05] dark:hover:border-brand-800"
                  }`}
                >
                  <button
                    type="button"
                    onClick={() => assignToSlot(face.codification.id)}
                    className="block w-full text-left"
                  >
                    <span className="block aspect-square w-full overflow-hidden bg-gray-100 dark:bg-white/[0.05]">
                      <img
                        src={face.thumbnailUrl}
                        alt={t("facial.faceAlt", { label: codificationLabel(face.codification) })}
                        className="h-full w-full object-cover"
                      />
                    </span>
                    <span className="block px-2 py-1.5">
                      <span className="block truncate text-xs font-medium text-gray-700 dark:text-gray-300">
                        {codificationLabel(face.codification)}
                      </span>
                    </span>
                  </button>

                  {slotIndex >= 0 && (
                    <span className="absolute left-1.5 top-1.5 flex h-5 w-5 items-center justify-center rounded bg-brand-500 text-xs font-medium text-white">
                      {slotIndex === 0 ? "A" : "B"}
                    </span>
                  )}

                  <button
                    type="button"
                    onClick={() => handleDeleteFace(face)}
                    disabled={deletingTraceId === face.codification.traceId}
                    title={t("facial.deleteThisFace")}
                    className="absolute right-1.5 top-1.5 hidden h-5 w-5 items-center justify-center rounded bg-black/60 text-xs text-white transition-colors hover:bg-error-500 disabled:opacity-50 group-hover:flex"
                  >
                    ×
                  </button>
                </div>
              );
            })}
          </div>
        )}
      </ComponentCard>

      <ComponentCard
        title={t("facial.comparison")}
        desc={t("facial.comparisonDesc")}
        collapsible
      >
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          <ComparePane
            caseId={caseId}
            slot="A"
            face={slotFaces[0]}
            onClear={() => clearSlot(0)}
          />
          <ComparePane
            caseId={caseId}
            slot="B"
            face={slotFaces[1]}
            onClear={() => clearSlot(1)}
          />
        </div>

        {currentComparison && (
          <MachineScore result={currentComparison} slotIds={slotIds} />
        )}

        <div className="flex flex-wrap items-center justify-between gap-3">
          <Button
            size="sm"
            onClick={handleMachineScore}
            disabled={!bothSlotsFilled || scoring}
          >
            {scoring ? t("facial.scoring") : t("facial.machineScore")}
          </Button>
          <div className="flex items-center gap-3">
            <button
              type="button"
              onClick={swapSlots}
              disabled={slotIds[0] === null && slotIds[1] === null}
              className="text-xs text-gray-500 hover:text-gray-700 disabled:opacity-40 dark:text-gray-400 dark:hover:text-gray-300"
            >
              {t("facial.swap")}
            </button>
            <button
              type="button"
              onClick={() => setSlotIds([null, null])}
              disabled={slotIds[0] === null && slotIds[1] === null}
              className="text-xs text-gray-500 hover:text-gray-700 disabled:opacity-40 dark:text-gray-400 dark:hover:text-gray-300"
            >
              {t("facial.clearBoth")}
            </button>
          </div>
        </div>
      </ComponentCard>

      <CaseClusters
        clusters={clusters}
        faces={faces}
        loading={clustersLoading}
        error={clustersError}
      />
    </div>
  );
}
