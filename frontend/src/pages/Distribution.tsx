import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import PageMeta from "../components/common/PageMeta";
import PageBreadcrumb from "../components/common/PageBreadCrumb";
import ComponentCard from "../components/common/ComponentCard";
import { Table, TableBody, TableCell, TableHeader, TableRow } from "../components/ui/table";
import Button from "../components/ui/button/Button";
import { typeKey } from "../components/analyses/labels";
import {
  distribute,
  getDistributionStatus,
  type AnalysisType,
  type DistributionResult,
  type DistributionStatus,
} from "../services/analyses";

const headerClass = "px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400";
const cellClass = "px-5 py-3 text-start text-theme-sm text-gray-700 dark:text-gray-300";
const inputClass =
  "h-11 rounded-lg border border-gray-200 bg-transparent px-4 py-2.5 text-sm text-gray-800 shadow-theme-xs focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-800 dark:bg-gray-900 dark:text-white/90";

/** Admin: what is waiting for an examiner, each examiner's load, and the automatic distribution of
 * a chosen number of items (previewed first), the rest staying pending. */
export default function Distribution() {
  const { t } = useTranslation();
  const [status, setStatus] = useState<DistributionStatus | null>(null);
  const [count, setCount] = useState(5);
  const [type, setType] = useState<AnalysisType | "">("");
  const [result, setResult] = useState<DistributionResult | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const load = useCallback(() => {
    getDistributionStatus()
      .then(setStatus)
      .catch((err) => setError(err instanceof Error ? err.message : t("analyses.loadError")));
  }, [t]);

  useEffect(load, [load]);

  async function run(dryRun: boolean) {
    setBusy(true);
    try {
      const res = await distribute(count, type, dryRun);
      setResult(res);
      if (!dryRun) {
        toast.success(t("distribution.done", { count: res.assigned.length }));
        load();
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("analyses.saveError"));
    } finally {
      setBusy(false);
    }
  }

  const totalPending = status?.pending.reduce((n, p) => n + p.count, 0) ?? 0;

  return (
    <>
      <PageMeta title={`${t("distribution.title")} | TrackID`} description={t("distribution.desc")} />
      <PageBreadcrumb pageTitle={t("distribution.title")} />
      {error && <p className="mb-4 text-sm text-error-500">{error}</p>}
      <div className="grid grid-cols-1 gap-6 xl:grid-cols-2">
        <ComponentCard title={t("distribution.pending")} desc={t("distribution.pendingDesc", { count: totalPending })}>
          {status && status.pending.length === 0 && (
            <p className="text-sm text-gray-500 dark:text-gray-400">{t("distribution.nothingPending")}</p>
          )}
          {status && status.pending.length > 0 && (
            <Table>
              <TableHeader className="border-b border-gray-100 dark:border-white/[0.05]">
                <TableRow>
                  <TableCell isHeader className={headerClass}>{t("distribution.columns.type")}</TableCell>
                  <TableCell isHeader className={headerClass}>{t("distribution.columns.stage")}</TableCell>
                  <TableCell isHeader className={headerClass}>{t("distribution.columns.count")}</TableCell>
                </TableRow>
              </TableHeader>
              <TableBody className="divide-y divide-gray-100 dark:divide-white/[0.05]">
                {status.pending.map((p) => (
                  <TableRow key={`${p.type}-${p.stage}`}>
                    <TableCell className={cellClass}>{t(typeKey(p.type))}</TableCell>
                    <TableCell className={cellClass}>{t(`analyses.stage.${p.stage}`)}</TableCell>
                    <TableCell className={cellClass}>{p.count}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </ComponentCard>

        <ComponentCard title={t("distribution.examiners")} desc={t("distribution.examinersDesc")}>
          {status && status.examiners.length === 0 && (
            <p className="text-sm text-gray-500 dark:text-gray-400">{t("distribution.noExaminers")}</p>
          )}
          {status && status.examiners.length > 0 && (
            <Table>
              <TableHeader className="border-b border-gray-100 dark:border-white/[0.05]">
                <TableRow>
                  <TableCell isHeader className={headerClass}>{t("distribution.columns.examiner")}</TableCell>
                  <TableCell isHeader className={headerClass}>{t("distribution.columns.modalities")}</TableCell>
                  <TableCell isHeader className={headerClass}>{t("distribution.columns.openWork")}</TableCell>
                </TableRow>
              </TableHeader>
              <TableBody className="divide-y divide-gray-100 dark:divide-white/[0.05]">
                {status.examiners.map((e) => (
                  <TableRow key={e.userId}>
                    <TableCell className={cellClass}>{e.username}</TableCell>
                    <TableCell className={cellClass}>
                      {[e.examinesFacial && t("modality.facial"), e.examinesFingerprint && t("modality.fingerprint")]
                        .filter(Boolean)
                        .join(", ")}
                    </TableCell>
                    <TableCell className={cellClass}>{e.openWork}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </ComponentCard>
      </div>

      <ComponentCard className="mt-6" title={t("distribution.run")} desc={t("distribution.runDesc")}>
        <div className="flex flex-wrap items-end gap-3">
          <label className="flex flex-col gap-1 text-sm text-gray-700 dark:text-gray-400">
            {t("distribution.count")}
            <input
              type="number"
              min={1}
              value={count}
              onChange={(event) => setCount(Math.max(1, Number(event.target.value) || 1))}
              className={`${inputClass} w-28`}
            />
          </label>
          <label className="flex flex-col gap-1 text-sm text-gray-700 dark:text-gray-400">
            {t("distribution.type")}
            <select value={type} onChange={(event) => setType(event.target.value as AnalysisType | "")} className={inputClass}>
              <option value="">{t("distribution.allTypes")}</option>
              <option value="FACIAL_REVIEW">{t(typeKey("FACIAL_REVIEW"))}</option>
              <option value="FACIAL_COMPARISON">{t(typeKey("FACIAL_COMPARISON"))}</option>
              <option value="FINGERPRINT_COMPARISON">{t(typeKey("FINGERPRINT_COMPARISON"))}</option>
            </select>
          </label>
          <Button size="sm" variant="outline" onClick={() => void run(true)} disabled={busy}>
            {t("distribution.preview")}
          </Button>
          <Button size="sm" onClick={() => void run(false)} disabled={busy}>
            {t("distribution.distribute")}
          </Button>
        </div>

        {result && (
          <div className="mt-6">
            <p className="mb-3 text-sm text-gray-600 dark:text-gray-400">
              {result.dryRun ? t("distribution.previewResult") : t("distribution.result")}{" "}
              {t("distribution.stillPending", { count: result.stillPending })}
              {result.noExaminer.length > 0 && ` ${t("distribution.noExaminer", { count: result.noExaminer.length })}`}
            </p>
            {result.assigned.length > 0 && (
              <Table>
                <TableHeader className="border-b border-gray-100 dark:border-white/[0.05]">
                  <TableRow>
                    <TableCell isHeader className={headerClass}>{t("analyses.columns.case")}</TableCell>
                    <TableCell isHeader className={headerClass}>{t("analyses.columns.analysis")}</TableCell>
                    <TableCell isHeader className={headerClass}>{t("distribution.columns.step")}</TableCell>
                    <TableCell isHeader className={headerClass}>{t("distribution.columns.examiner")}</TableCell>
                  </TableRow>
                </TableHeader>
                <TableBody className="divide-y divide-gray-100 dark:divide-white/[0.05]">
                  {result.assigned.map((a) => (
                    <TableRow key={`${a.analysisId}-${a.step}`}>
                      <TableCell className={cellClass}>
                        <Link to={`/cases/${encodeURIComponent(a.caseId)}`} className="text-brand-500 hover:underline">
                          {a.caseId}
                        </Link>
                      </TableCell>
                      <TableCell className={cellClass}>
                        {t(typeKey(a.type))} #{a.sequence}
                      </TableCell>
                      <TableCell className={cellClass}>{t(`analyses.step.${a.step}`)}</TableCell>
                      <TableCell className={cellClass}>{a.username}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </div>
        )}
      </ComponentCard>
    </>
  );
}
