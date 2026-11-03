import { useEffect, useState } from "react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import { Modal } from "../ui/modal";
import Button from "../ui/button/Button";
import TraceEditor, { type TraceBox } from "./TraceEditor";
import CodificationEditorModal from "./CodificationEditorModal";
import {
  createTraces,
  deleteTrace,
  getEvidenceObjectUrl,
  listTraces,
  type Trace,
} from "../../services/cases";

interface TraceMarkerModalProps {
  caseId: string;
  /** Forwarded to CodificationEditorModal, which uses it to decide whether
   * to offer manual point marking (FINGERPRINT) or just image adjustment
   * (FACIAL) -- see that component. */
  modality: string;
  evidenceId: number;
  filename: string;
  isOpen: boolean;
  onClose: () => void;
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

export default function TraceMarkerModal({
  caseId,
  modality,
  evidenceId,
  filename,
  isOpen,
  onClose,
}: TraceMarkerModalProps) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [imageUrl, setImageUrl] = useState<string | null>(null);
  const [traces, setTraces] = useState<TraceBox[]>([]);
  const [saving, setSaving] = useState(false);
  const [deletingId, setDeletingId] = useState<string | null>(null);
  const [codificationTrace, setCodificationTrace] = useState<TraceBox | null>(
    null,
  );

  useEffect(() => {
    if (!isOpen) return;
    let cancelled = false;

    setLoading(true);
    setError("");
    setTraces([]);

    Promise.all([
      getEvidenceObjectUrl(caseId, evidenceId),
      listTraces(caseId, evidenceId),
    ])
      .then(([url, existing]) => {
        if (cancelled) {
          URL.revokeObjectURL(url);
          return;
        }
        setImageUrl(url);
        setTraces(existing.map(toLockedBox));
      })
      .catch((err) => {
        if (!cancelled) {
          setError(
            err instanceof Error ? err.message : t("markTraces.loadError"),
          );
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [isOpen, caseId, evidenceId, t]);

  useEffect(() => {
    return () => {
      if (imageUrl) URL.revokeObjectURL(imageUrl);
    };
  }, [imageUrl]);

  const newTraces = traces.filter((t) => !t.locked);
  const lockedTraces = traces.filter((t) => t.locked);

  async function handleDeleteLocked(trace: TraceBox) {
    const traceId = Number(trace.id);
    if (!Number.isFinite(traceId)) return;
    const number = traces.findIndex((t) => t.id === trace.id) + 1;
    if (!window.confirm(t("markTraces.confirmDelete", { number }))) {
      return;
    }
    setDeletingId(trace.id);
    try {
      await deleteTrace(caseId, evidenceId, traceId);
      setTraces((current) => current.filter((t) => t.id !== trace.id));
      toast.success(t("markTraces.deleted"));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("markTraces.deleteError"));
    } finally {
      setDeletingId(null);
    }
  }

  async function handleSave() {
    if (newTraces.length === 0) return;
    setSaving(true);
    try {
      const created = await createTraces(
        caseId,
        evidenceId,
        newTraces.map((t) => ({
          boxX1: t.boxX1,
          boxY1: t.boxY1,
          boxX2: t.boxX2,
          boxY2: t.boxY2,
        })),
      );
      setTraces((current) => [
        ...current.filter((t) => t.locked),
        ...created.map(toLockedBox),
      ]);
      toast.success(t("markTraces.saved", { count: created.length }));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("markTraces.saveError"));
    } finally {
      setSaving(false);
    }
  }

  return (
    <Modal isOpen={isOpen} onClose={onClose} className="m-4 max-w-6xl">
      <div className="p-4">
        <h4 className="mb-1 text-lg font-medium text-gray-800 dark:text-white/90">
          {t("caseDetail.markTraces")}
        </h4>
        <p className="mb-4 text-sm text-gray-500 dark:text-gray-400">
          {filename}
        </p>

        {loading && (
          <p className="py-16 text-center text-sm text-gray-500 dark:text-gray-400">
            {t("common.loadingShort")}
          </p>
        )}

        {!loading && error && (
          <p className="py-16 text-center text-sm text-error-500">{error}</p>
        )}

        {!loading && !error && imageUrl && (
          <>
            <TraceEditor
              src={imageUrl}
              traces={traces}
              onChange={setTraces}
              className="h-[70vh] w-full rounded-xl"
            />

            {lockedTraces.length > 0 && (
              <div className="mt-4">
                <p className="mb-2 text-xs font-medium text-gray-500 dark:text-gray-400">
                  {t("markTraces.savedTraces")}
                </p>
                <ul className="divide-y divide-gray-100 rounded-lg border border-gray-200 dark:divide-white/[0.05] dark:border-white/[0.05]">
                  {lockedTraces.map((trace) => (
                    <li
                      key={trace.id}
                      className="flex items-center justify-between px-3 py-2 text-sm"
                    >
                      <span className="text-gray-700 dark:text-gray-300">
                        {t("markTraces.traceNumber", { number: traces.findIndex((x) => x.id === trace.id) + 1 })}
                      </span>
                      <span className="flex items-center gap-3">
                        <button
                          type="button"
                          onClick={() => setCodificationTrace(trace)}
                          className="text-xs text-brand-500 hover:text-brand-600"
                        >
                          {t("markTraces.editCodification")}
                        </button>
                        <button
                          type="button"
                          onClick={() => handleDeleteLocked(trace)}
                          disabled={deletingId === trace.id}
                          className="text-xs text-error-500 hover:text-error-600 disabled:opacity-50 dark:text-error-400"
                        >
                          {deletingId === trace.id ? t("markTraces.deleting") : t("common.delete")}
                        </button>
                      </span>
                    </li>
                  ))}
                </ul>
              </div>
            )}

            <div className="mt-4 flex items-center justify-between">
              <p className="text-xs text-gray-500 dark:text-gray-400">
                {t("markTraces.total", { count: traces.length })}
                {newTraces.length > 0 &&
                  ` — ${t("markTraces.unsaved", { count: newTraces.length })}`}
              </p>
              <div className="flex gap-2">
                <Button size="sm" variant="outline" onClick={onClose}>
                  {t("common.close")}
                </Button>
                <Button
                  size="sm"
                  onClick={handleSave}
                  disabled={newTraces.length === 0 || saving}
                >
                  {saving ? t("common.saving") : t("markTraces.saveTraces")}
                </Button>
              </div>
            </div>
          </>
        )}
      </div>

      {codificationTrace && (
        <CodificationEditorModal
          caseId={caseId}
          modality={modality}
          evidenceId={evidenceId}
          traceId={Number(codificationTrace.id)}
          traceLabel={t("markTraces.traceNumber", { number: traces.findIndex((x) => x.id === codificationTrace.id) + 1 })}
          box={{
            x1: codificationTrace.boxX1,
            y1: codificationTrace.boxY1,
            x2: codificationTrace.boxX2,
            y2: codificationTrace.boxY2,
          }}
          isOpen={!!codificationTrace}
          onClose={() => setCodificationTrace(null)}
        />
      )}
    </Modal>
  );
}
