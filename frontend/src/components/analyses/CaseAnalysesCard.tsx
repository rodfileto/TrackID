import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import ComponentCard from "../common/ComponentCard";
import Badge from "../ui/badge/Badge";
import Button from "../ui/button/Button";
import { Modal } from "../ui/modal";
import { Table, TableBody, TableCell, TableHeader, TableRow } from "../ui/table";
import { stageColor, typeKey } from "./labels";
import { useCurrentUser } from "../../context/CurrentUserContext";
import {
  activeStep,
  analysisSteps,
  assignAnalysis,
  getWorksheet,
  listCaseAnalyses,
  listUsers,
  openAnalysis,
  type Analysis,
  type PairView,
} from "../../services/analyses";
import type { UserProfile } from "../../services/auth";

const headerClass = "px-4 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400";
const cellClass = "px-4 py-3 text-start text-theme-sm text-gray-700 dark:text-gray-300";
const selectClass =
  "h-9 rounded-lg border border-gray-200 bg-transparent px-2 text-sm text-gray-800 dark:border-gray-800 dark:bg-gray-900 dark:text-white/90";

/** Admin: give an open analysis's current step to an eligible examiner, or back to pending. */
function AssignControl({ analysis, users, onDone }: { analysis: Analysis; users: UserProfile[]; onDone: () => void }) {
  const { t } = useTranslation();
  const step = activeStep(analysis.stage);
  const current = analysis.examiners[step]?.userId;
  const others = analysisSteps.filter((s) => s !== step).map((s) => analysis.examiners[s]?.userId);
  const eligible = users.filter((u) => u.is_active && u.examines_facial && !others.includes(u.id));
  const [choice, setChoice] = useState<string>(current ? String(current) : "");

  async function save() {
    try {
      await assignAnalysis(analysis.id, choice ? Number(choice) : null);
      toast.success(t("analyses.assigned"));
      onDone();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("analyses.saveError"));
    }
  }

  return (
    <div className="flex items-center gap-2">
      <select value={choice} onChange={(e) => setChoice(e.target.value)} className={selectClass}>
        <option value="">{t("analyses.unassigned")}</option>
        {eligible.map((u) => (
          <option key={u.id} value={u.id}>
            {u.username}
          </option>
        ))}
      </select>
      <Button size="sm" variant="outline" onClick={() => void save()} disabled={choice === (current ? String(current) : "")}>
        {t("analyses.assign", { step: t(`analyses.step.${step}`) })}
      </Button>
    </div>
  );
}

/** Admin: open a FACIAL_COMPARISON on pairs picked from the case's review. */
function OpenComparisonModal({
  caseId,
  review,
  isOpen,
  onClose,
  onDone,
}: {
  caseId: string;
  review: Analysis | undefined;
  isOpen: boolean;
  onClose: () => void;
  onDone: () => void;
}) {
  const { t } = useTranslation();
  const [pairs, setPairs] = useState<PairView[]>([]);
  const [selected, setSelected] = useState<Set<string>>(new Set());

  useEffect(() => {
    if (!isOpen || !review) return;
    setSelected(new Set());
    getWorksheet(caseId, review.id)
      .then((ws) => setPairs(ws.pairs))
      .catch(() => setPairs([]));
  }, [isOpen, caseId, review]);

  function toggle(key: string) {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  }

  async function create() {
    try {
      const chosen = pairs.filter((p) => selected.has(`${p.featureA}|${p.featureB}`));
      await openAnalysis(caseId, "FACIAL_COMPARISON", chosen.map((p) => ({ featureA: p.featureA, featureB: p.featureB })));
      toast.success(t("analyses.comparisonOpened"));
      onClose();
      onDone();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("analyses.saveError"));
    }
  }

  return (
    <Modal isOpen={isOpen} onClose={onClose} className="max-w-2xl m-4">
      <div className="p-6">
        <h3 className="mb-1 text-lg font-semibold text-gray-800 dark:text-white/90">{t("analyses.openComparison")}</h3>
        <p className="mb-4 text-sm text-gray-500 dark:text-gray-400">{t("analyses.openComparisonDesc")}</p>
        {pairs.length === 0 ? (
          <p className="text-sm text-gray-500 dark:text-gray-400">{t("analyses.noPairs")}</p>
        ) : (
          <ul className="max-h-[50vh] space-y-2 overflow-y-auto">
            {pairs.map((p) => {
              const key = `${p.featureA}|${p.featureB}`;
              const other = p.counterpart;
              return (
                <li key={key}>
                  <label className="flex cursor-pointer items-center gap-3 rounded-lg border border-gray-200 px-3 py-2 text-sm dark:border-gray-800">
                    <input type="checkbox" checked={selected.has(key)} onChange={() => toggle(key)} />
                    <span className="text-gray-700 dark:text-gray-300">
                      {t("analyses.caseFace", { traceId: p.questioned.traceId, caseId })} ×{" "}
                      {other.kind === "KNOWN"
                        ? t("analyses.enrolled", { name: other.name, personId: other.personId })
                        : t("analyses.caseFace", { traceId: other.traceId, caseId: other.caseId })}
                    </span>
                    {p.score !== undefined && <span className="ml-auto text-gray-500">{p.score.toFixed(3)}</span>}
                  </label>
                </li>
              );
            })}
          </ul>
        )}
        <div className="mt-5 flex justify-end gap-2">
          <Button size="sm" variant="outline" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button size="sm" onClick={() => void create()} disabled={selected.size === 0}>
            {t("analyses.openComparisonButton", { count: selected.size })}
          </Button>
        </div>
      </div>
    </Modal>
  );
}

/** A case's analyses: each one's stage, examiners and report, a link to work on it, and, for
 * admins, assigning its current step and opening a comparison. */
export default function CaseAnalysesCard({ caseId }: { caseId: string }) {
  const { t } = useTranslation();
  const { isAdmin } = useCurrentUser();
  const [list, setList] = useState<Analysis[] | null>(null);
  const [users, setUsers] = useState<UserProfile[]>([]);
  const [comparing, setComparing] = useState(false);
  const [error, setError] = useState("");

  const load = useCallback(() => {
    listCaseAnalyses(caseId)
      .then(setList)
      .catch((err) => setError(err instanceof Error ? err.message : t("analyses.loadError")));
  }, [caseId, t]);

  useEffect(load, [load]);
  useEffect(() => {
    if (isAdmin) listUsers().then(setUsers).catch(() => setUsers([]));
  }, [isAdmin]);

  const review = list?.find((a) => a.type === "FACIAL_REVIEW");

  return (
    <ComponentCard title={t("analyses.title")} desc={t("analyses.desc")}>
      {error && <p className="text-sm text-error-500">{error}</p>}
      {list && list.length === 0 && <p className="text-sm text-gray-500 dark:text-gray-400">{t("analyses.none")}</p>}
      {list && list.length > 0 && (
        <div className="overflow-x-auto">
          <Table>
            <TableHeader className="border-b border-gray-100 dark:border-white/[0.05]">
              <TableRow>
                <TableCell isHeader className={headerClass}>{t("analyses.columns.analysis")}</TableCell>
                <TableCell isHeader className={headerClass}>{t("analyses.columns.stage")}</TableCell>
                <TableCell isHeader className={headerClass}>{t("analyses.columns.examiners")}</TableCell>
                <TableCell isHeader className={headerClass}>{t("analyses.columns.report")}</TableCell>
                <TableCell isHeader className={headerClass}>{""}</TableCell>
              </TableRow>
            </TableHeader>
            <TableBody className="divide-y divide-gray-100 dark:divide-white/[0.05]">
              {list.map((a) => (
                <TableRow key={a.id}>
                  <TableCell className={cellClass}>
                    <div className="font-medium">
                      {t(typeKey(a.type))} #{a.sequence}
                    </div>
                    <div className="text-theme-xs text-gray-500">
                      {a.requestedBy ? t("analyses.requestedBy", { name: a.requestedBy }) : t("analyses.automatic")}
                      {a.pairs.length > 0 && ` · ${t("analyses.pairsCount", { count: a.pairs.length })}`}
                    </div>
                  </TableCell>
                  <TableCell className={cellClass}>
                    <Badge size="sm" color={stageColor(a.stage, a.status)}>
                      {a.status === "CLOSED" ? t("analyses.closed") : t(`analyses.stage.${a.stage}`)}
                    </Badge>
                  </TableCell>
                  <TableCell className={cellClass}>
                    <ul className="space-y-0.5 text-theme-xs">
                      {analysisSteps.map((s) =>
                        a.examiners[s] ? (
                          <li key={s}>
                            {t(`analyses.step.${s}`)}: {a.examiners[s]?.username}
                          </li>
                        ) : null,
                      )}
                      {!a.examiners.VERIFICATOR && <li className="text-gray-400">{t("analyses.pending")}</li>}
                    </ul>
                    {isAdmin && a.status === "OPEN" && (
                      <div className="mt-2">
                        <AssignControl analysis={a} users={users} onDone={load} />
                      </div>
                    )}
                  </TableCell>
                  <TableCell className={cellClass}>
                    {a.report.status === "ISSUED" ? (
                      <Badge size="sm" color="success">
                        {t("analyses.reportNumber", { number: a.report.number })}
                      </Badge>
                    ) : (
                      <span className="text-theme-xs text-gray-500">{t("analyses.reportDraft")}</span>
                    )}
                    <div className="text-theme-xs text-gray-500">{t(`analyses.reportType.${a.report.type}`)}</div>
                  </TableCell>
                  <TableCell className={cellClass}>
                    <Link
                      to={`/cases/${encodeURIComponent(caseId)}/analyses/${a.id}`}
                      className="text-sm font-medium text-brand-500 hover:underline"
                    >
                      {t("analyses.open")}
                    </Link>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
      {isAdmin && review && (
        <div className="mt-4">
          <Button size="sm" variant="outline" onClick={() => setComparing(true)}>
            {t("analyses.openComparison")}
          </Button>
        </div>
      )}
      <OpenComparisonModal caseId={caseId} review={review} isOpen={comparing} onClose={() => setComparing(false)} onDone={load} />
    </ComponentCard>
  );
}
