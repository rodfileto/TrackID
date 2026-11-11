import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import ComponentCard from "../common/ComponentCard";
import FileDropzone from "../common/FileDropzone";
import { IMAGE_FILES } from "../common/fileTypes";
import Badge from "../ui/badge/Badge";
import CaseTraceThumbnail from "../cases/CaseTraceThumbnail";
import TraceMarkerModal from "../cases/TraceMarkerModal";
import { addEvidence, getCase, type Evidence } from "../../services/cases";
import { listCaseMaterial, setTraceRole, type MaterialTrace } from "../../services/requests";
import { addExaminerPairs, removeExaminerPair, type PairView } from "../../services/analyses";

interface Props {
  caseId: string;
  analysisId: number;
  /** The case's traces (listCaseMaterial). */
  material: MaterialTrace[];
  pairs: PairView[];
  /** Reloads the worksheet and the material after a change. */
  onChanged: () => Promise<void> | void;
}

const selectClass =
  "h-11 w-full rounded-lg border border-gray-300 bg-transparent px-3 text-sm text-gray-800 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90";
const buttonClass =
  "h-9 rounded-lg border border-gray-300 px-3 text-sm font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-50 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-white/[0.03]";

const isImage = (e: Evidence) => e.category === "evidence" && (e.mediaType === "image" || (e.contentType ?? "").startsWith("image/"));

// The material of a fingerprint examination, for its first examiner while they examine
// (docs/scene-examination-plan.md, decision 6): mark the lifts on the case's evidence, bring in
// reference documents (a fingerprint card, an identity document), and list the pairs to compare.
// Each pair is then decided below as any pair is, and its text goes to the laudo.
export default function AnalysisMaterialPanel({ caseId, analysisId, material, pairs, onChanged }: Props) {
  const { t } = useTranslation();
  const [evidences, setEvidences] = useState<Evidence[]>([]);
  const [marking, setMarking] = useState<Evidence | null>(null);
  const [uploading, setUploading] = useState(false);
  const [questioned, setQuestioned] = useState("");
  const [counterpart, setCounterpart] = useState("");
  const [busy, setBusy] = useState(false);

  const loadEvidences = useCallback(async () => {
    try {
      setEvidences((await getCase(caseId)).evidences.filter(isImage));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : String(err));
    }
  }, [caseId]);

  useEffect(() => {
    void loadEvidences();
  }, [loadEvidences]);

  const tracesOf = useMemo(() => {
    const out = new Map<number, MaterialTrace[]>();
    for (const m of material) {
      if (!m.evidenceFileId) continue;
      out.set(m.evidenceFileId, [...(out.get(m.evidenceFileId) ?? []), m]);
    }
    return out;
  }, [material]);

  const broughtIn = (e: Evidence) => e.analysisId === analysisId;

  function originLabel(e: Evidence): string {
    if (broughtIn(e)) return t("analysisMaterial.broughtIn");
    if (e.collection) return t("analysisMaterial.fromCollection", { sequence: e.collection.sequence });
    if (e.request) return t("analysisMaterial.fromRequest", { document: e.request.documentReference });
    return t("analysisMaterial.noOrigin");
  }

  // Traces marked on a document this analysis brought in are references: set them so when the
  // marker closes.
  async function closeMarker() {
    const evidence = marking;
    setMarking(null);
    await onChanged();
    if (evidence && broughtIn(evidence)) {
      const traces = (await listCaseMaterial(caseId)).filter((m) => m.evidenceFileId === evidence.id && m.role === "QUESTIONED");
      for (const m of traces) {
        try {
          await setTraceRole(caseId, m.traceId, "REFERENCE");
        } catch {
          // A trace already decided on keeps its role; the list shows it.
        }
      }
      if (traces.length > 0) await onChanged();
    }
  }

  async function bringIn(files: File[]) {
    setUploading(true);
    try {
      let last: Evidence | null = null;
      // The reply describes the file as stored, before its origin is recorded: it came from here.
      for (const file of files) last = { ...(await addEvidence(caseId, file, { analysisId })), analysisId };
      await loadEvidences();
      toast.success(t("analysisMaterial.broughtInDone", { count: files.length }));
      // Straight to marking the one just brought in, when there was one.
      if (files.length === 1 && last) setMarking(last);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : String(err));
    } finally {
      setUploading(false);
    }
  }

  async function changeRole(m: MaterialTrace, role: MaterialTrace["role"]) {
    try {
      await setTraceRole(caseId, m.traceId, role);
      await onChanged();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : String(err));
    }
  }

  async function addPair() {
    if (!questioned || !counterpart) return;
    setBusy(true);
    try {
      await addExaminerPairs(caseId, analysisId, [{ featureA: questioned, featureB: counterpart }]);
      setCounterpart("");
      await onChanged();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  async function removePair(p: PairView) {
    if (!window.confirm(t("analysisMaterial.confirmRemovePair"))) return;
    try {
      await removeExaminerPair(caseId, analysisId, { featureA: p.featureA, featureB: p.featureB });
      await onChanged();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : String(err));
    }
  }

  const lifts = material.filter((m) => m.role === "QUESTIONED");
  const references = material.filter((m) => m.role === "REFERENCE");
  const label = (m: MaterialTrace) => `${m.role === "REFERENCE" ? t("analysisMaterial.reference") : t("analysisMaterial.lift")} ${m.label}`;
  const counterparts = material.filter((m) => m.featureId !== questioned);
  const byFeature = new Map(material.map((m) => [m.featureId, m]));
  const sideLabel = (featureId: string) => {
    const m = byFeature.get(featureId);
    return m ? label(m) : featureId;
  };

  return (
    <ComponentCard title={t("analysisMaterial.title")} desc={t("analysisMaterial.desc")}>
      <div className="flex flex-col gap-3">
        {evidences.length === 0 && <p className="text-sm text-gray-500 dark:text-gray-400">{t("analysisMaterial.noEvidence")}</p>}
        <ul className="flex flex-col divide-y divide-gray-100 dark:divide-white/[0.05]">
          {evidences.map((e) => {
            const traces = tracesOf.get(e.id) ?? [];
            return (
              <li key={e.id} className="flex flex-col gap-2 py-3">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="text-sm font-medium text-gray-800 dark:text-white/90">{e.filename}</span>
                  <Badge size="sm" color={broughtIn(e) ? "primary" : "light"}>{originLabel(e)}</Badge>
                  {e.vestige && <Badge size="sm" color="light">{e.vestige.label}</Badge>}
                  <button type="button" className={`${buttonClass} ml-auto`} onClick={() => setMarking(e)}>
                    {t("analysisMaterial.mark")}
                  </button>
                </div>
                {traces.length > 0 && (
                  <div className="flex flex-wrap gap-3">
                    {traces.map((m) => (
                      <div key={m.traceId} className="flex items-center gap-2 rounded-lg border border-gray-200 p-2 dark:border-gray-800">
                        <CaseTraceThumbnail caseId={caseId} thumbnailFileId={m.cropFileId ?? m.evidenceFileId}
                          thumbnailBox={m.cropFileId ? undefined : m.box} alt={label(m)} />
                        <div className="flex flex-col gap-1">
                          <span className="text-xs font-medium text-gray-700 dark:text-gray-300">{label(m)}</span>
                          <select aria-label={t("analysisMaterial.role")} className="h-8 rounded-md border border-gray-300 bg-transparent px-2 text-xs dark:border-gray-700 dark:bg-gray-900"
                            value={m.role} onChange={(ev) => void changeRole(m, ev.target.value as MaterialTrace["role"])}>
                            <option value="QUESTIONED">{t("analysisMaterial.lift")}</option>
                            <option value="REFERENCE">{t("analysisMaterial.reference")}</option>
                          </select>
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </li>
            );
          })}
        </ul>

        <div>
          <p className="mb-2 text-sm font-medium text-gray-700 dark:text-gray-300">{t("analysisMaterial.bringIn")}</p>
          <FileDropzone id="analysis-reference" multiple variant="compact" accept={IMAGE_FILES} disabled={uploading}
            description={t("analysisMaterial.bringInHint")} onFiles={(files) => void bringIn(files)} />
        </div>

        <div className="flex flex-col gap-3 border-t border-gray-100 pt-4 dark:border-white/[0.05]">
          <p className="text-sm font-medium text-gray-700 dark:text-gray-300">{t("analysisMaterial.pairsTitle")}</p>
          {lifts.length === 0 ? (
            <p className="text-sm text-gray-500 dark:text-gray-400">{t("analysisMaterial.markFirst")}</p>
          ) : (
            <div className="grid grid-cols-1 items-end gap-3 md:grid-cols-[1fr_1fr_auto]">
              <label className="text-sm text-gray-700 dark:text-gray-300">
                {t("analysisMaterial.lift")}
                <select className={selectClass} value={questioned} onChange={(e) => setQuestioned(e.target.value)}>
                  <option value="">—</option>
                  {lifts.map((m) => <option key={m.featureId} value={m.featureId}>{label(m)}</option>)}
                </select>
              </label>
              <label className="text-sm text-gray-700 dark:text-gray-300">
                {t("analysisMaterial.comparedWith")}
                <select className={selectClass} value={counterpart} onChange={(e) => setCounterpart(e.target.value)}>
                  <option value="">—</option>
                  {references.filter((m) => m.featureId !== questioned).map((m) => <option key={m.featureId} value={m.featureId}>{label(m)}</option>)}
                  {counterparts.filter((m) => m.role === "QUESTIONED").map((m) => <option key={m.featureId} value={m.featureId}>{label(m)}</option>)}
                </select>
              </label>
              <button type="button" className="h-11 rounded-lg bg-brand-500 px-4 text-sm font-medium text-white hover:bg-brand-600 disabled:opacity-50"
                disabled={!questioned || !counterpart || busy} onClick={() => void addPair()}>
                {t("analysisMaterial.addPair")}
              </button>
            </div>
          )}
          {pairs.length > 0 && (
            <ul className="flex flex-col gap-1 text-sm text-gray-700 dark:text-gray-300">
              {pairs.map((p) => (
                <li key={`${p.featureA}|${p.featureB}`} className="flex items-center gap-3">
                  <span>{sideLabel(p.featureA)} × {sideLabel(p.featureB)}</span>
                  {!p.draft && p.decisions.length === 0 && (
                    <button type="button" className="text-error-500 hover:underline" onClick={() => void removePair(p)}>
                      {t("common.remove")}
                    </button>
                  )}
                </li>
              ))}
            </ul>
          )}
        </div>
      </div>

      {marking && (
        <TraceMarkerModal caseId={caseId} modality="FINGERPRINT" evidenceId={marking.id} filename={marking.filename} isOpen
          onClose={() => void closeMarker()} />
      )}
    </ComponentCard>
  );
}
