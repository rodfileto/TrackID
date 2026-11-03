import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import ComponentCard from "../common/ComponentCard";
import Badge from "../ui/badge/Badge";
import Button from "../ui/button/Button";
import ReportEditor from "./ReportEditor";
import {
  deliverReport,
  downloadReport,
  getReport,
  issueReport,
  refreshReportDraft,
  type ReportOverview,
} from "../../services/analyses";

const inputClass =
  "h-10 w-full rounded-lg border border-gray-300 bg-transparent px-3 text-sm text-gray-800 shadow-theme-xs focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90";

function resultColor(code: string): "success" | "error" | "warning" | "info" | "light" {
  switch (code) {
    case "IDENTIFIED":
      return "success";
    case "EXCLUDED":
      return "error";
    case "LEAD":
    case "SUPPORTED":
      return "info";
    case "UNSETTLED":
      return "light";
  }
  return "warning";
}

/** An analysis's report: its executive summary (preliminary until issued), the editor for its
 * draft, issuing, the documents and their deliveries. canWrite: the viewer is the first examiner
 * at the REPORT stage. canDeliver: a coordinator or the issuer. */
export default function ReportPanel({
  caseId,
  analysisId,
  canWrite,
  canDeliver,
  onIssued,
}: {
  caseId: string;
  analysisId: number;
  canWrite: boolean;
  canDeliver: boolean;
  onIssued: () => void;
}) {
  const { t } = useTranslation();
  const [report, setReport] = useState<ReportOverview | null>(null);
  const [editing, setEditing] = useState(false);
  const [busy, setBusy] = useState(false);
  const [delivery, setDelivery] = useState({ deliveredOn: new Date().toISOString().slice(0, 10), method: "", recipient: "" });

  const load = useCallback(async () => {
    try {
      setReport(await getReport(caseId, analysisId));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : String(err));
    }
  }, [caseId, analysisId]);

  useEffect(() => {
    void load();
  }, [load]);

  async function run(action: () => Promise<void>) {
    setBusy(true);
    try {
      await action();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  if (!report) return null;
  const s = report.summary;
  const issued = report.status === "ISSUED";
  const fields: [string, string][] = [
    ["document", `${s.title}${s.number ? ` nº ${s.number}` : ""}`],
    ["case", s.caseId],
    ["occurrence", s.occurrence || "—"],
    ["request", s.request ? `${s.request} (${s.questions})` : "—"],
    ["material", s.material],
    ["references", s.references],
    ["verification", s.verification],
    ["previous", s.previous.join("; ") || "—"],
    ["examiners", s.examiners],
  ];

  return (
    <ComponentCard title={t("reports.title")}>
      <div className="space-y-5">
        <div className="flex flex-wrap items-center gap-2">
          <h4 className="text-sm font-semibold text-gray-800 dark:text-white/90">{t("reports.summary")}</h4>
          {s.preliminary ? (
            <Badge size="sm" color="warning">
              {t("reports.preliminary")}
            </Badge>
          ) : (
            <Badge size="sm" color="success">
              {s.number}
            </Badge>
          )}
        </div>
        <dl className="grid grid-cols-1 gap-x-6 gap-y-2 text-sm sm:grid-cols-[12rem_1fr]">
          {fields.map(([key, value]) => (
            <div key={key} className="contents">
              <dt className="text-gray-500 dark:text-gray-400">{t(`reports.fields.${key}`)}</dt>
              <dd className="text-gray-800 dark:text-white/90">{value}</dd>
            </div>
          ))}
        </dl>

        {s.results && s.results.length > 0 && (
          <div className="overflow-x-auto">
            <table className="min-w-full text-sm">
              <thead>
                <tr className="text-left text-gray-500 dark:text-gray-400">
                  <th className="py-1 pr-4">{t("reports.columns.questioned")}</th>
                  <th className="py-1 pr-4">{t("reports.columns.reference")}</th>
                  <th className="py-1 pr-4">{t("reports.columns.conclusion")}</th>
                  <th className="py-1 pr-4">{t("reports.columns.result")}</th>
                  <th className="py-1">{t("reports.columns.lead")}</th>
                </tr>
              </thead>
              <tbody className="text-gray-800 dark:text-white/90">
                {s.results.map((r, i) => (
                  <tr key={i} className="border-t border-gray-100 dark:border-gray-800">
                    <td className="py-1.5 pr-4">{r.questioned}</td>
                    <td className="py-1.5 pr-4">{r.counterpart}</td>
                    <td className="py-1.5 pr-4">{r.level}</td>
                    <td className="py-1.5 pr-4">
                      <Badge size="sm" color={resultColor(r.resultCode)}>
                        {r.result}
                      </Badge>
                    </td>
                    <td className="py-1.5">{r.lead}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        {s.faces && s.faces.length > 0 && (
          <div className="overflow-x-auto">
            <table className="min-w-full text-sm">
              <thead>
                <tr className="text-left text-gray-500 dark:text-gray-400">
                  <th className="py-1 pr-4">{t("reports.columns.face")}</th>
                  <th className="py-1 pr-4">{t("reports.columns.quality")}</th>
                  <th className="py-1 pr-4">{t("reports.columns.result")}</th>
                  <th className="py-1">{t("reports.columns.leads")}</th>
                </tr>
              </thead>
              <tbody className="text-gray-800 dark:text-white/90">
                {s.faces.map((f, i) => (
                  <tr key={i} className="border-t border-gray-100 dark:border-gray-800">
                    <td className="py-1.5 pr-4">{f.face}</td>
                    <td className="py-1.5 pr-4">{f.suitability}</td>
                    <td className="py-1.5 pr-4">
                      <Badge size="sm" color={resultColor(f.resultCode)}>
                        {f.result}
                      </Badge>
                    </td>
                    <td className="py-1.5">{f.leads}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        {s.notice && <p className="rounded-lg border border-warning-300 p-3 text-sm font-medium text-warning-700 dark:text-warning-400">{s.notice}</p>}

        {!issued && canWrite && (
          <div className="space-y-2">
            {report.required.length > 0 && (
              <p className="text-sm text-gray-500 dark:text-gray-400">{t("reports.required", { sections: report.required.join(", ") })}</p>
            )}
            <div className="flex flex-wrap gap-2">
              <Button size="sm" onClick={() => setEditing(true)} disabled={busy}>
                {t("reports.edit")}
              </Button>
              {report.draft && (
                <Button
                  size="sm"
                  variant="outline"
                  disabled={busy}
                  onClick={() =>
                    void run(async () => {
                      const v = await refreshReportDraft(caseId, analysisId);
                      toast.success(t("reports.refreshed", { version: v.version }));
                      await load();
                    })
                  }
                >
                  {t("reports.refresh")}
                </Button>
              )}
              <Button
                size="sm"
                variant="outline"
                disabled={busy}
                onClick={() =>
                  void run(async () => {
                    if (!window.confirm(t("reports.issueConfirm"))) return;
                    const r = await issueReport(caseId, analysisId);
                    toast.success(t("reports.issued", { number: r.number }));
                    await load();
                    onIssued();
                  })
                }
              >
                {t("reports.issue")}
              </Button>
            </div>
          </div>
        )}

        {issued && (
          <div className="space-y-4">
            <div className="flex flex-wrap items-center gap-2">
              {(["pdf", "docx"] as const).map((format) => (
                <Button key={format} size="sm" variant="outline" onClick={() => void run(() => downloadReport(caseId, analysisId, format))}>
                  {t("reports.download", { format: format.toUpperCase() })}
                </Button>
              ))}
              {report.pdfSha256 && (
                <span className="break-all font-mono text-xs text-gray-500 dark:text-gray-400">{t("reports.hash", { hash: report.pdfSha256 })}</span>
              )}
            </div>
            <div>
              <h4 className="mb-2 text-sm font-semibold text-gray-800 dark:text-white/90">{t("reports.deliveries")}</h4>
              {report.deliveries.length === 0 ? (
                <p className="text-sm text-gray-500 dark:text-gray-400">{t("reports.noDeliveries")}</p>
              ) : (
                <ul className="space-y-1 text-sm text-gray-700 dark:text-gray-300">
                  {report.deliveries.map((d, i) => (
                    <li key={i}>
                      {d.deliveredOn} — {d.method} — {d.recipient}
                      {d.notes ? ` (${d.notes})` : ""} · {d.recordedBy}
                    </li>
                  ))}
                </ul>
              )}
              {canDeliver && (
                <form
                  className="mt-3 grid grid-cols-1 gap-2 sm:grid-cols-[10rem_1fr_1fr_auto]"
                  onSubmit={(event) => {
                    event.preventDefault();
                    void run(async () => {
                      setReport(await deliverReport(caseId, analysisId, delivery));
                      setDelivery((d) => ({ ...d, method: "", recipient: "" }));
                      toast.success(t("reports.delivered"));
                    });
                  }}
                >
                  <input type="date" required aria-label={t("reports.deliveredOn")} className={inputClass} value={delivery.deliveredOn}
                    onChange={(e) => setDelivery({ ...delivery, deliveredOn: e.target.value })} />
                  <input required placeholder={t("reports.method")} className={inputClass} value={delivery.method}
                    onChange={(e) => setDelivery({ ...delivery, method: e.target.value })} />
                  <input required placeholder={t("reports.recipient")} className={inputClass} value={delivery.recipient}
                    onChange={(e) => setDelivery({ ...delivery, recipient: e.target.value })} />
                  <Button size="sm" type="submit" disabled={busy}>
                    {t("reports.deliver")}
                  </Button>
                </form>
              )}
            </div>
          </div>
        )}
      </div>
      {editing && (
        <ReportEditor
          caseId={caseId}
          analysisId={analysisId}
          title={s.title}
          onClose={() => {
            setEditing(false);
            void load();
          }}
        />
      )}
    </ComponentCard>
  );
}
