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
  getDistributionStatus,
  getPriorDeciders,
  getWorksheet,
  listCaseAnalyses,
  openAnalysis,
  addAnalysisPairs,
  traceKey,
  type Analysis,
  type AnalysisType,
  type DistributionExaminer,
  type FeaturePair,
  type PairView,
} from "../../services/analyses";
import { listCaseMaterial, listCaseRequests, type CaseRequest, type MaterialTrace } from "../../services/requests";

const headerClass = "px-4 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400";
const cellClass = "px-4 py-3 text-start text-theme-sm text-gray-700 dark:text-gray-300";
const selectClass =
  "h-9 rounded-lg border border-gray-200 bg-transparent px-2 text-sm text-gray-800 dark:border-gray-800 dark:bg-gray-900 dark:text-white/90";

/** Coordinator: give an open analysis's current step to an eligible examiner, or back to pending.
 * users are the active examiners, as distribution lists them. Those who already decided the step's
 * pairs elsewhere are marked, with a warning when one is chosen: distribution avoids them, and a
 * manual assignment should too unless nobody else can take it. */
function AssignControl({
  analysis,
  users,
  onDone,
}: {
  analysis: Analysis;
  users: DistributionExaminer[];
  onDone: () => void;
}) {
  const { t } = useTranslation();
  const step = activeStep(analysis.stage);
  const current = analysis.examiners[step]?.userId;
  const others = analysisSteps.filter((s) => s !== step).map((s) => analysis.examiners[s]?.userId);
  const eligible = users.filter((u) => u.examinesFacial && !others.includes(u.userId));
  const [choice, setChoice] = useState<string>(current ? String(current) : "");
  const [prior, setPrior] = useState<number[]>([]);

  useEffect(() => {
    getPriorDeciders(analysis.id)
      .then(setPrior)
      .catch(() => setPrior([]));
  }, [analysis.id, analysis.stage]);
  const warn = choice !== "" && choice !== (current ? String(current) : "") && prior.includes(Number(choice));

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
    <div className="flex flex-col gap-1">
      <div className="flex items-center gap-2">
        <select value={choice} onChange={(e) => setChoice(e.target.value)} className={selectClass}>
          <option value="">{t("analyses.unassigned")}</option>
          {eligible.map((u) => (
            <option key={u.userId} value={u.userId}>
              {prior.includes(u.userId) ? t("analyses.priorDeciderOption", { name: u.username }) : u.username}
            </option>
          ))}
        </select>
        <Button size="sm" variant="outline" onClick={() => void save()} disabled={choice === (current ? String(current) : "")}>
          {t("analyses.assign", { step: t(`analyses.step.${step}`) })}
        </Button>
      </div>
      {warn && <p className="max-w-xs text-theme-xs text-warning-600 dark:text-warning-400">{t("analyses.priorDeciderWarning")}</p>}
    </div>
  );
}

function ordered(a: string, b: string): FeaturePair {
  return a < b ? { featureA: a, featureB: b } : { featureA: b, featureB: a };
}

/** Coordinator: choose a comparison's pairs -- from the case's review (faces), or between a
 * questioned trace and a reference sent in -- and add them to the comparison open at its
 * examination stage (the one the request opened), or open a new one for one of the case's requests,
 * optionally as a laudo complementar of an issued laudo. A questioned trace not assessed as suitable
 * for comparison is flagged: the comparison may not be able to conclude. */
function OpenComparisonModal({
  caseId,
  modality,
  review,
  analyses,
  requests,
  isOpen,
  onClose,
  onDone,
}: {
  caseId: string;
  modality: string;
  review: Analysis | undefined;
  analyses: Analysis[];
  requests: CaseRequest[];
  isOpen: boolean;
  onClose: () => void;
  onDone: () => void;
}) {
  const { t } = useTranslation();
  const [pairs, setPairs] = useState<PairView[]>([]);
  const [material, setMaterial] = useState<MaterialTrace[]>([]);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [requestId, setRequestId] = useState<string>("");
  const [complements, setComplements] = useState<string>("");
  const type: AnalysisType = modality === "FINGERPRINT" ? "FINGERPRINT_COMPARISON" : "FACIAL_COMPARISON";
  const target = [...analyses].reverse().find((a) => a.type === type && a.status === "OPEN" && a.stage === "EXAMINATION");
  const [into, setInto] = useState<string>("new");

  useEffect(() => {
    if (!isOpen) return;
    setInto(target ? String(target.id) : "new");
    setSelected(new Set());
    setRequestId(requests.length ? String(requests[requests.length - 1].id) : "");
    setComplements("");
    listCaseMaterial(caseId).then(setMaterial).catch(() => setMaterial([]));
    if (!review) return;
    getWorksheet(caseId, review.id)
      .then((ws) => setPairs(ws.pairs))
      .catch(() => setPairs([]));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isOpen, caseId, review, requests]);

  const byFeature = new Map(material.map((m) => [m.featureId, m]));
  const questioned = material.filter((m) => m.role === "QUESTIONED");
  const references = material.filter((m) => m.role === "REFERENCE");
  const local = questioned.flatMap((q) => references.map((r) => ({ ...ordered(q.featureId, r.featureId), q, r })));
  const issuedLaudos = analyses.filter((a) => a.type === type && a.report.status === "ISSUED");
  const unsuitable = (featureId?: string) => {
    const m = featureId ? byFeature.get(featureId) : undefined;
    return !!m && m.role === "QUESTIONED" && m.suitability !== "SUITABLE";
  };

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
      const chosen = [...pairs, ...local].filter((p) => selected.has(`${p.featureA}|${p.featureB}`));
      const unique = [...new Map(chosen.map((p) => [`${p.featureA}|${p.featureB}`, { featureA: p.featureA, featureB: p.featureB }])).values()];
      if (into !== "new") {
        await addAnalysisPairs(caseId, Number(into), unique);
        toast.success(t("analyses.pairsAdded", { count: unique.length }));
      } else {
        await openAnalysis(caseId, type, unique, {
          requestId: requestId ? Number(requestId) : undefined,
          complements: complements ? Number(complements) : undefined,
        });
        toast.success(t("analyses.comparisonOpened"));
      }
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
        {target && (
          <select value={into} onChange={(e) => setInto(e.target.value)} className={`${selectClass} mb-3`}>
            <option value={target.id}>{t("analyses.addTo", { type: t(typeKey(target.type)), sequence: target.sequence })}</option>
            <option value="new">{t("analyses.newComparison")}</option>
          </select>
        )}
        <div className={`mb-4 grid grid-cols-1 gap-3 sm:grid-cols-2 ${into !== "new" ? "hidden" : ""}`}>
          {requests.length > 0 && (
            <select value={requestId} onChange={(e) => setRequestId(e.target.value)} className={selectClass}>
              {requests.map((r) => (
                <option key={r.id} value={r.id}>
                  {t("requests.sequence", { sequence: r.sequence })} — {r.documentReference}
                </option>
              ))}
            </select>
          )}
          {issuedLaudos.length > 0 && (
            <select value={complements} onChange={(e) => setComplements(e.target.value)} className={selectClass}>
              <option value="">—</option>
              {issuedLaudos.map((a) => (
                <option key={a.id} value={a.id}>
                  {t("analyses.complements", { number: a.report.number })}
                </option>
              ))}
            </select>
          )}
        </div>
        {local.length > 0 && (
          <ul className="mb-3 space-y-2">
            {local.map((p) => {
              const key = `${p.featureA}|${p.featureB}`;
              return (
                <li key={key}>
                  <label className="flex cursor-pointer items-center gap-3 rounded-lg border border-gray-200 px-3 py-2 text-sm dark:border-gray-800">
                    <input type="checkbox" checked={selected.has(key)} onChange={() => toggle(key)} />
                    <span className="text-gray-700 dark:text-gray-300">
                      {p.q.label} × {p.r.label} ({t("requests.role.REFERENCE")})
                    </span>
                    {unsuitable(p.q.featureId) && (
                      <Badge size="sm" color="warning">
                        {t(`analyses.suitability.${p.q.suitability ?? "none"}`)}
                      </Badge>
                    )}
                  </label>
                </li>
              );
            })}
          </ul>
        )}
        {pairs.length === 0 && local.length === 0 ? (
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
                      {t(traceKey(modality), { traceId: p.questioned.traceId, caseId })} ×{" "}
                      {other.kind === "KNOWN"
                        ? t("analyses.enrolled", { name: other.name, personId: other.personId })
                        : t(traceKey(modality), { traceId: other.traceId, caseId: other.caseId })}
                    </span>
                    {unsuitable(p.questioned.featureId) && (
                      <Badge size="sm" color="warning">
                        {t(`analyses.suitability.${byFeature.get(p.questioned.featureId ?? "")?.suitability ?? "none"}`)}
                      </Badge>
                    )}
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
            {into !== "new"
              ? t("analyses.addPairsButton", { count: selected.size })
              : t("analyses.openComparisonButton", { count: selected.size })}
          </Button>
        </div>
      </div>
    </Modal>
  );
}

/** A case's analyses: each one's stage, examiners and report, a link to work on it, and, for
 * coordinators, assigning its current step and opening a comparison. refreshKey reloads the list when
 * an analysis is opened elsewhere on the page (e.g. from the case's clusters). */
export default function CaseAnalysesCard({ caseId, modality, refreshKey = 0 }: { caseId: string; modality: string; refreshKey?: number }) {
  const { t } = useTranslation();
  const { isCoordinator } = useCurrentUser();
  const [list, setList] = useState<Analysis[] | null>(null);
  const [users, setUsers] = useState<DistributionExaminer[]>([]);
  const [comparing, setComparing] = useState(false);
  const [error, setError] = useState("");
  const [requests, setRequests] = useState<CaseRequest[]>([]);

  const load = useCallback(() => {
    listCaseAnalyses(caseId)
      .then(setList)
      .catch((err) => setError(err instanceof Error ? err.message : t("analyses.loadError")));
    listCaseRequests(caseId)
      .then(setRequests)
      .catch(() => setRequests([]));
  }, [caseId, t]);
  const requestSequence = new Map(requests.map((r) => [r.id, r.sequence]));

  useEffect(load, [load, refreshKey]);
  useEffect(() => {
    if (isCoordinator)
      getDistributionStatus()
        .then((s) => setUsers(s.examiners))
        .catch(() => setUsers([]));
  }, [isCoordinator]);

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
                    {a.requestId && requestSequence.has(a.requestId) && (
                      <div className="text-theme-xs text-gray-500">
                        {t("analyses.answersRequest", { sequence: requestSequence.get(a.requestId) })}
                      </div>
                    )}
                    {a.report.complements && (
                      <div className="text-theme-xs text-gray-500">{t("analyses.complements", { number: a.report.complements })}</div>
                    )}
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
                    {isCoordinator && a.status === "OPEN" && (
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
      {isCoordinator && list && (
        <div className="mt-4">
          <Button size="sm" variant="outline" onClick={() => setComparing(true)}>
            {t("analyses.choosePairs")}
          </Button>
        </div>
      )}
      <OpenComparisonModal
        caseId={caseId}
        modality={modality}
        review={review}
        analyses={list ?? []}
        requests={requests}
        isOpen={comparing}
        onClose={() => setComparing(false)}
        onDone={load}
      />
    </ComponentCard>
  );
}
