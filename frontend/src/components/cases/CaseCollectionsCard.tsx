import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import ComponentCard from "../common/ComponentCard";
import Badge from "../ui/badge/Badge";
import { useCurrentUser } from "../../context/CurrentUserContext";
import { listCaseRequests, type CaseRequest } from "../../services/requests";
import {
  addVestige,
  breakSeal,
  getCaseOrigins,
  openFollowUp,
  reseal,
  setReportMode,
  type CaseOrigins,
  type Collection,
  type ReportMode,
  type Vestige,
} from "../../services/collections";

const inputClass =
  "h-9 rounded-lg border border-gray-300 bg-transparent px-3 text-sm text-gray-800 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90";
const smallButton =
  "inline-flex h-8 items-center rounded-lg border border-gray-200 bg-white px-3 text-xs font-medium text-gray-700 shadow-theme-xs hover:bg-gray-50 disabled:opacity-50 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-300 dark:hover:bg-white/[0.03]";

function when(iso: string): string {
  return new Date(iso).toLocaleString();
}

/** Where a case's material came from (docs/scene-collection-plan.md): the scenes examined on call,
 * each awaiting or formalized by its request, and the physical items, received or collected, with
 * their seals. Whoever works the case, or a coordinator, adds items and breaks or applies seals. */
export default function CaseCollectionsCard({
  caseId,
  modality,
  canWrite,
  refreshKey,
  onChanged,
}: {
  caseId: string;
  modality: string;
  canWrite: boolean;
  refreshKey: number;
  onChanged: () => void;
}) {
  const { t } = useTranslation();
  const { isCoordinator, isFingerprintExaminer } = useCurrentUser();
  const [origins, setOrigins] = useState<CaseOrigins | null>(null);
  const [requests, setRequests] = useState<CaseRequest[]>([]);
  const [busy, setBusy] = useState(false);
  const [draft, setDraft] = useState({ description: "", sealNumber: "", origin: "" });
  const canChange = canWrite || isCoordinator;
  // A coordinator holds no analysis of the case: the coordinator's routes, not the case's.
  const asCoordinator = !canWrite && isCoordinator;

  const load = useCallback(() => {
    Promise.all([getCaseOrigins(caseId), listCaseRequests(caseId)])
      .then(([o, r]) => {
        setOrigins(o);
        setRequests(r);
      })
      .catch(() => setOrigins(null));
  }, [caseId]);

  useEffect(load, [load, refreshKey]);

  async function act(action: () => Promise<unknown>, done: string) {
    setBusy(true);
    try {
      await action();
      toast.success(done);
      load();
      onChanged();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  if (!origins) return null;
  const collections = origins.collections;
  const vestiges = origins.vestiges;
  const requestLabel = (id?: number) => {
    const r = requests.find((x) => x.id === id);
    return r ? t("origin.request", { sequence: r.sequence, document: r.documentReference }) : "";
  };
  const collectionLabel = (id?: number) => {
    const c = collections.find((x) => x.id === id);
    return c ? t("collections.collectionN", { sequence: c.sequence }) : "";
  };
  // Nothing to show to someone who cannot add anything.
  if (collections.length === 0 && vestiges.length === 0 && !canChange) return null;

  function addDraftVestige() {
    const [kind, id] = [draft.origin.slice(0, 1), Number(draft.origin.slice(1))];
    void act(
      () =>
        addVestige(
          caseId,
          { description: draft.description, sealNumber: draft.sealNumber || undefined, ...(kind === "r" ? { requestId: id } : { collectionId: id }) },
          asCoordinator,
        ).then(() => setDraft({ description: "", sealNumber: "", origin: draft.origin })),
      t("collections.vestigeAdded"),
    );
  }

  function askReseal(v: Vestige) {
    const number = window.prompt(t("collections.resealPrompt", { label: v.label }));
    if (number && number.trim()) void act(() => reseal(caseId, v.id, number.trim(), asCoordinator), t("collections.sealed"));
  }

  function askBreak(v: Vestige) {
    if (window.confirm(t("collections.breakConfirm", { label: v.label, number: v.seals[v.seals.length - 1]?.number }))) {
      void act(() => breakSeal(caseId, v.id, asCoordinator), t("collections.broken"));
    }
  }

  function comparisonFor(c: Collection) {
    if (!c.request) return;
    const request = c.request;
    void act(() => openFollowUp(caseId, "FINGERPRINT_COMPARISON", request.id), t("collections.comparisonOpened"));
  }

  const originChoices = [
    ...requests.map((r) => ({ key: `r${r.id}`, label: t("origin.request", { sequence: r.sequence, document: r.documentReference }) })),
    ...collections.filter((c) => !c.nothingCollected).map((c) => ({ key: `c${c.id}`, label: t("collections.collectionN", { sequence: c.sequence }) })),
  ];

  return (
    <ComponentCard title={t("collections.title")} desc={t("collections.desc")}>
      <div className="flex flex-wrap items-center gap-3 text-sm text-gray-700 dark:text-gray-300">
        <span>{t("collections.reportMode")}:</span>
        {isCoordinator ? (
          <select
            className={inputClass}
            value={origins.reportMode}
            disabled={busy}
            onChange={(e) => void act(() => setReportMode(caseId, e.target.value as ReportMode), t("collections.reportModeSaved"))}
          >
            <option value="SEPARATE">{t("reportMode.SEPARATE")}</option>
            <option value="TOGETHER">{t("reportMode.TOGETHER")}</option>
          </select>
        ) : (
          <span className="font-medium">{t(`reportMode.${origins.reportMode}`)}</span>
        )}
      </div>

      {collections.length > 0 && (
        <ul className="divide-y divide-gray-100 dark:divide-white/[0.05]">
          {collections.map((c) => (
            <li key={c.id} className="flex flex-wrap items-start justify-between gap-3 py-3 text-sm">
              <div className="space-y-1">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="font-medium text-gray-800 dark:text-white/90">{t("collections.collectionN", { sequence: c.sequence })}</span>
                  {c.request ? (
                    <Badge size="sm" color="success">{t("collections.formalized")}</Badge>
                  ) : (
                    <Badge size="sm" color="warning">{t("collections.awaiting")}</Badge>
                  )}
                  {c.nothingCollected && <Badge size="sm" color="light">{t("collections.nothingCollected")}</Badge>}
                </div>
                <p className="text-gray-600 dark:text-gray-400">
                  {t("collections.collectedBy", { by: c.collectedBy, from: when(c.collectedFrom) })}
                  {c.collectedTo && ` – ${when(c.collectedTo)}`}
                  {c.registeredBy && c.registeredBy !== c.collectedBy && ` · ${t("collections.registeredBy", { by: c.registeredBy })}`}
                </p>
                {c.place && <p className="text-gray-600 dark:text-gray-400">{c.place}</p>}
                {c.notes && <p className="text-gray-600 dark:text-gray-400">{c.notes}</p>}
                {c.request && (
                  <p className="text-gray-600 dark:text-gray-400">
                    {t("collections.formalizedBy", { document: c.request.documentReference, date: c.request.receivedOn })}
                  </p>
                )}
              </div>
              {c.request && !c.nothingCollected && modality === "FINGERPRINT" && canWrite && isFingerprintExaminer && (
                <button type="button" className={smallButton} disabled={busy} onClick={() => comparisonFor(c)}>
                  {t("collections.openComparison")}
                </button>
              )}
            </li>
          ))}
        </ul>
      )}

      <div>
        <h4 className="mb-2 text-sm font-medium text-gray-800 dark:text-white/90">{t("collections.vestiges")}</h4>
        {vestiges.length === 0 ? (
          <p className="text-sm text-gray-500 dark:text-gray-400">{t("collections.noVestiges")}</p>
        ) : (
          <ul className="divide-y divide-gray-100 dark:divide-white/[0.05]">
            {vestiges.map((v) => {
              const current = v.seals.find((s) => !s.brokenAt);
              return (
                <li key={v.id} className="flex flex-wrap items-start justify-between gap-3 py-3 text-sm">
                  <div className="space-y-1">
                    <p>
                      <span className="font-medium text-gray-800 dark:text-white/90">{v.label}</span>{" "}
                      <span className="text-gray-700 dark:text-gray-300">{v.description}</span>
                    </p>
                    <p className="text-xs text-gray-500 dark:text-gray-400">
                      {v.requestId ? t("collections.receivedWith", { request: requestLabel(v.requestId) }) : t("collections.collectedIn", { collection: collectionLabel(v.collectionId) })}
                      {v.askedBy.length > 0 && ` · ${t("collections.askedBy", { requests: v.askedBy.map(requestLabel).join(", ") })}`}
                    </p>
                    {v.seals.length === 0 ? (
                      <p className="text-xs text-gray-500 dark:text-gray-400">{t("collections.unsealed")}</p>
                    ) : (
                      <ol className="space-y-0.5 text-xs text-gray-500 dark:text-gray-400">
                        {v.seals.map((s) => (
                          <li key={s.id}>
                            <span className={s.brokenAt ? "line-through" : "font-medium text-gray-700 dark:text-gray-300"}>
                              {t("collections.seal", { number: s.number })}
                            </span>{" "}
                            {t("collections.sealApplied", { at: when(s.appliedAt), by: s.appliedBy || t("collections.outside") })}
                            {s.brokenAt && ` · ${t("collections.sealBroken", { at: when(s.brokenAt), by: s.brokenBy })}`}
                          </li>
                        ))}
                      </ol>
                    )}
                  </div>
                  {canChange && (
                    <div className="flex gap-2">
                      {current && (
                        <button type="button" className={smallButton} disabled={busy} onClick={() => askBreak(v)}>
                          {t("collections.breakSeal")}
                        </button>
                      )}
                      <button type="button" className={smallButton} disabled={busy} onClick={() => askReseal(v)}>
                        {t("collections.reseal")}
                      </button>
                    </div>
                  )}
                </li>
              );
            })}
          </ul>
        )}
      </div>

      {canChange && originChoices.length > 0 && (
        <div className="flex flex-wrap items-end gap-2 border-t border-gray-100 pt-4 dark:border-white/[0.05]">
          <label className="text-xs text-gray-600 dark:text-gray-400">
            {t("collections.newVestige")}
            <input className={`${inputClass} block w-64`} value={draft.description} placeholder={t("collections.vestigePlaceholder")}
              onChange={(e) => setDraft({ ...draft, description: e.target.value })} />
          </label>
          <label className="text-xs text-gray-600 dark:text-gray-400">
            {t("collections.sealNumber")}
            <input className={`${inputClass} block w-36`} value={draft.sealNumber} onChange={(e) => setDraft({ ...draft, sealNumber: e.target.value })} />
          </label>
          <label className="text-xs text-gray-600 dark:text-gray-400">
            {t("origin.label")}
            <select className={`${inputClass} block`} value={draft.origin} onChange={(e) => setDraft({ ...draft, origin: e.target.value })}>
              <option value="">{t("origin.choose")}</option>
              {originChoices.map((o) => (
                <option key={o.key} value={o.key}>{o.label}</option>
              ))}
            </select>
          </label>
          <button type="button" className={smallButton} disabled={busy || !draft.description.trim() || !draft.origin} onClick={addDraftVestige}>
            {t("collections.addVestige")}
          </button>
        </div>
      )}
    </ComponentCard>
  );
}
