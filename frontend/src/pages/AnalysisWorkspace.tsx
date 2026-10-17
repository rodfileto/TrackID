import { useCallback, useEffect, useState } from "react";
import { Link, useParams } from "react-router";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import PageMeta from "../components/common/PageMeta";
import PageBreadcrumb from "../components/common/PageBreadCrumb";
import ComponentCard from "../components/common/ComponentCard";
import Badge from "../components/ui/badge/Badge";
import Button from "../components/ui/button/Button";
import FaceImage from "../components/analyses/FaceImage";
import { decisionColor, decisionValues, stageColor, typeKey } from "../components/analyses/labels";
import { useCurrentUser } from "../context/CurrentUserContext";
import {
  getWorksheet,
  issueReport,
  listCaseAnalyses,
  saveDraft,
  submitStep,
  type Analysis,
  type DecisionValue,
  type PairView,
  type Worksheet,
} from "../services/analyses";
import type { TraceLink } from "../services/cases";

function pairKey(p: { featureA: string; featureB: string }): string {
  return `${p.featureA}|${p.featureB}`;
}

function describe(face: TraceLink, t: (key: string, opts?: Record<string, unknown>) => string): string {
  if (face.kind === "KNOWN") {
    return t("analyses.enrolled", { name: face.name ?? "?", personId: face.personId ?? "?" });
  }
  return t("analyses.caseFace", { traceId: face.traceId ?? "?", caseId: face.caseId ?? "?" });
}

/** One pair: the two faces side by side, what the matcher said, the decisions the viewer may see,
 * and -- when it is their step -- their own decision and notes. Blind (verification) shows only the
 * two images and the decision: no score, no matcher decision, nothing naming who or which case is
 * on them. */
function PairCard({
  pair,
  index,
  editable,
  blind,
  onDecide,
}: {
  pair: PairView;
  index: number;
  editable: boolean;
  blind: boolean;
  onDecide: (pair: PairView, decision: DecisionValue, notes: string) => Promise<void>;
}) {
  const { t } = useTranslation();
  const [notes, setNotes] = useState(pair.draft?.notes ?? "");
  const [saving, setSaving] = useState(false);

  async function decide(decision: DecisionValue) {
    setSaving(true);
    try {
      await onDecide(pair, decision, notes);
    } finally {
      setSaving(false);
    }
  }

  return (
    <div
      className={`rounded-2xl border p-4 ${
        pair.draft
          ? "border-gray-200 dark:border-gray-800"
          : editable
            ? "border-brand-300 dark:border-brand-800"
            : "border-gray-200 dark:border-gray-800"
      }`}
    >
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <span className="text-sm font-medium text-gray-800 dark:text-white/90">
          {t("analyses.pairNumber", { n: index + 1 })}
        </span>
        {!blind && pair.score !== undefined && (
          <Badge size="sm" color="light">
            {t("analyses.score", { score: pair.score.toFixed(3) })}
          </Badge>
        )}
        {!blind && pair.systemDecision && (
          <Badge size="sm" color={decisionColor(pair.systemDecision)}>
            {t("analyses.system")}: {t(`analyses.decision.${pair.systemDecision}`)}
          </Badge>
        )}
        {!blind && pair.disputed && (
          <Badge size="sm" color="warning">
            {t("analyses.disputed")}
          </Badge>
        )}
      </div>

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <FaceImage face={pair.questioned} label={blind ? t("analyses.blindQuestioned") : describe(pair.questioned, t)} />
        <FaceImage face={pair.counterpart} label={blind ? t("analyses.blindReference") : describe(pair.counterpart, t)} />
      </div>

      {!blind && pair.decisions.length > 0 && (
        <ul className="mt-3 space-y-1">
          {pair.decisions.map((d) => (
            <li key={d.step} className="flex flex-wrap items-center gap-2 text-sm text-gray-600 dark:text-gray-400">
              <span>{t(`analyses.step.${d.step}`)}</span>
              <Badge size="sm" color={decisionColor(d.decision)}>
                {t(`analyses.decision.${d.decision}`)}
              </Badge>
              <span>{d.username}</span>
              {d.notes && <span className="italic">“{d.notes}”</span>}
            </li>
          ))}
        </ul>
      )}

      {editable && (
        <div className="mt-4 space-y-3">
          <textarea
            rows={2}
            value={notes}
            onChange={(event) => setNotes(event.target.value)}
            onBlur={() => {
              if (pair.draft && notes !== (pair.draft.notes ?? "")) void decide(pair.draft.decision);
            }}
            placeholder={t("analyses.notesPlaceholder")}
            className="w-full rounded-lg border border-gray-300 bg-transparent px-4 py-2.5 text-sm text-gray-800 shadow-theme-xs focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90"
          />
          <div className="flex flex-wrap gap-2">
            {decisionValues.map((value) => (
              <button
                key={value}
                type="button"
                disabled={saving}
                onClick={() => void decide(value)}
                className={`rounded-lg border px-4 py-2 text-sm font-medium transition-colors disabled:opacity-50 ${
                  pair.draft?.decision === value
                    ? value === "POSITIVE"
                      ? "border-success-500 bg-success-500 text-white"
                      : value === "NEGATIVE"
                        ? "border-error-500 bg-error-500 text-white"
                        : "border-warning-500 bg-warning-500 text-white"
                    : "border-gray-300 text-gray-700 hover:bg-gray-50 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-white/[0.03]"
                }`}
              >
                {t(`analyses.decision.${value}`)}
              </button>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

export default function AnalysisWorkspace() {
  const { t } = useTranslation();
  const { caseId = "", analysisId = "" } = useParams<{ caseId: string; analysisId: string }>();
  const id = Number(analysisId);
  const { user } = useCurrentUser();
  const [worksheet, setWorksheet] = useState<Worksheet | null>(null);
  const [analysis, setAnalysis] = useState<Analysis | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      const [ws, list] = await Promise.all([getWorksheet(caseId, id), listCaseAnalyses(caseId)]);
      setWorksheet(ws);
      setAnalysis(list.find((a) => a.id === id) ?? null);
      setError("");
    } catch (err) {
      setError(err instanceof Error ? err.message : t("analyses.loadError"));
    }
  }, [caseId, id, t]);

  useEffect(() => {
    void load();
  }, [load]);

  async function onDecide(pair: PairView, decision: DecisionValue, notes: string) {
    try {
      await saveDraft(caseId, id, pair, decision, notes);
      setWorksheet((ws) => {
        if (!ws) return ws;
        const pairs = ws.pairs.map((p) =>
          pairKey(p) === pairKey(pair) ? { ...p, draft: { decision, notes } } : p,
        );
        return { ...ws, pairs, drafted: pairs.filter((p) => p.draft).length };
      });
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("analyses.saveError"));
    }
  }

  async function onSubmit() {
    if (!window.confirm(t("analyses.submitConfirm"))) return;
    setBusy(true);
    try {
      const result = await submitStep(caseId, id);
      toast.success(t("analyses.submitted", { stage: t(`analyses.stage.${result.stage}`) }));
      await load();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("analyses.saveError"));
    } finally {
      setBusy(false);
    }
  }

  async function onIssue() {
    if (!window.confirm(t("analyses.issueConfirm"))) return;
    setBusy(true);
    try {
      const report = await issueReport(caseId, id);
      toast.success(t("analyses.issued", { number: report.number }));
      await load();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("analyses.saveError"));
    } finally {
      setBusy(false);
    }
  }

  // Blind verification: the verifier sees only the images and makes the decision -- no case, no
  // links to it or its clusters, no scores (FISWG: verification without contextual information).
  const blind = worksheet?.yourStep === "REVIEWER";
  const title = blind
    ? t("analyses.blindTitle")
    : analysis
    ? t("analyses.workspaceTitle", { type: t(typeKey(analysis.type)), sequence: analysis.sequence, caseId })
    : t("analyses.workspaceTitleShort", { caseId });
  const editable = !!worksheet?.yourStep;
  const canIssue =
    !!analysis &&
    analysis.status === "OPEN" &&
    analysis.stage === "REPORT" &&
    analysis.examiners.VERIFICATOR?.userId === user?.id;

  return (
    <>
      <PageMeta title={`${title} | TrackID`} description={t("analyses.metaDescription")} />
      <PageBreadcrumb pageTitle={title} />

      <div className="mb-6">
        {blind ? (
          <Link to="/analyses" className="text-sm text-gray-500 hover:text-gray-700 dark:text-gray-400">
            ← {t("analyses.backToMyAnalyses")}
          </Link>
        ) : (
          <Link to={`/cases/${encodeURIComponent(caseId)}`} className="text-sm text-gray-500 hover:text-gray-700 dark:text-gray-400">
            ← {t("analyses.backToCase", { caseId })}
          </Link>
        )}
      </div>

      {error && <p className="text-sm text-error-500">{error}</p>}

      {worksheet && (
        <div className="space-y-6">
          <ComponentCard title={t("analyses.status")}>
            <div className="flex flex-wrap items-center gap-3">
              <Badge size="sm" color={stageColor(worksheet.stage, worksheet.status)}>
                {worksheet.status === "CLOSED" ? t("analyses.closed") : t(`analyses.stage.${worksheet.stage}`)}
              </Badge>
              {worksheet.yourStep && (
                <Badge size="sm" color="primary">
                  {t("analyses.yourStep", { step: t(`analyses.step.${worksheet.yourStep}`) })}
                </Badge>
              )}
              {!blind && analysis?.report.number && (
                <Badge size="sm" color="success">
                  {t("analyses.reportNumber", { number: analysis.report.number })}
                </Badge>
              )}
              {editable && (
                <span className="text-sm text-gray-600 dark:text-gray-400">
                  {t("analyses.progress", { drafted: worksheet.drafted, total: worksheet.pairs.length })}
                </span>
              )}
              <div className="ml-auto flex gap-2">
                {editable && (
                  <Button size="sm" onClick={() => void onSubmit()} disabled={busy || worksheet.drafted < worksheet.pairs.length}>
                    {t("analyses.submit")}
                  </Button>
                )}
                {canIssue && (
                  <Button size="sm" onClick={() => void onIssue()} disabled={busy}>
                    {t("analyses.issueReport")}
                  </Button>
                )}
              </div>
            </div>
            {worksheet.yourStep === "REVIEWER" && (
              <p className="mt-3 text-sm text-gray-500 dark:text-gray-400">{t("analyses.blindHint")}</p>
            )}
            {worksheet.yourStep === "INCONSISTENCE" && (
              <p className="mt-3 text-sm text-gray-500 dark:text-gray-400">{t("analyses.inconsistenceHint")}</p>
            )}
            {!editable && !canIssue && worksheet.status === "OPEN" && (
              <p className="mt-3 text-sm text-gray-500 dark:text-gray-400">{t("analyses.readOnlyHint")}</p>
            )}
          </ComponentCard>

          {worksheet.pairs.length === 0 ? (
            <p className="text-sm text-gray-500 dark:text-gray-400">{t("analyses.noPairs")}</p>
          ) : (
            worksheet.pairs.map((pair, index) => (
              <PairCard key={pairKey(pair)} pair={pair} index={index} editable={editable} blind={blind} onDecide={onDecide} />
            ))
          )}
        </div>
      )}
    </>
  );
}
