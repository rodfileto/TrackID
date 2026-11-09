import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { listCaseRequests, type CaseRequest } from "../../services/requests";
import { getCaseOrigins, originChosen, type CaseOrigins, type MaterialOrigin } from "../../services/collections";

const selectClass =
  "h-9 w-full rounded-lg border border-gray-300 bg-transparent px-2 text-xs text-gray-800 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90";

/** Where uploaded material came from (docs/scene-collection-plan.md): one of the case's requests or
 * its scene collections, and the vestige it was taken from. With a single choice it is taken. */
export default function MaterialOriginPicker({
  caseId,
  value,
  onChange,
  refreshKey,
  compact,
}: {
  caseId: string;
  value: MaterialOrigin | null;
  onChange: (origin: MaterialOrigin | null) => void;
  refreshKey?: number;
  compact?: boolean;
}) {
  const { t } = useTranslation();
  const [requests, setRequests] = useState<CaseRequest[]>([]);
  const [origins, setOrigins] = useState<CaseOrigins | null>(null);

  useEffect(() => {
    let cancelled = false;
    Promise.all([listCaseRequests(caseId), getCaseOrigins(caseId)])
      .then(([r, o]) => {
        if (cancelled) return;
        setRequests(r);
        setOrigins(o);
        const collections = o.collections.filter((c) => !c.nothingCollected);
        if (!originChosen(value)) {
          if (r.length === 1 && collections.length === 0) onChange({ requestId: r[0].id });
          else if (r.length === 0 && collections.length === 1) onChange({ collectionId: collections[0].id });
        }
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
    // value and onChange are read once per load: a choice made already is kept.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [caseId, refreshKey]);

  const collections = (origins?.collections ?? []).filter((c) => !c.nothingCollected);
  const key = value?.requestId ? `r${value.requestId}` : value?.collectionId ? `c${value.collectionId}` : "";
  const vestiges = (origins?.vestiges ?? []).filter((v) =>
    value?.requestId ? v.requestId === value.requestId : value?.collectionId ? v.collectionId === value.collectionId : false,
  );

  if (origins && requests.length === 0 && collections.length === 0) {
    return <p className="text-xs text-warning-600 dark:text-warning-400">{t("origin.none")}</p>;
  }

  function choose(k: string) {
    if (k.startsWith("r")) onChange({ requestId: Number(k.slice(1)) });
    else if (k.startsWith("c")) onChange({ collectionId: Number(k.slice(1)) });
    else onChange(null);
  }

  return (
    <div className={compact ? "flex flex-col gap-2" : "grid grid-cols-1 gap-2 sm:grid-cols-2"}>
      <label className="block text-xs text-gray-600 dark:text-gray-400">
        {t("origin.label")}
        <select className={selectClass} value={key} onChange={(e) => choose(e.target.value)}>
          <option value="">{t("origin.choose")}</option>
          {requests.map((r) => (
            <option key={`r${r.id}`} value={`r${r.id}`}>
              {t("origin.request", { sequence: r.sequence, document: r.documentReference })}
            </option>
          ))}
          {collections.map((c) => (
            <option key={`c${c.id}`} value={`c${c.id}`}>
              {t("origin.collection", { sequence: c.sequence, date: new Date(c.collectedFrom).toLocaleDateString(), by: c.collectedBy })}
            </option>
          ))}
        </select>
      </label>
      {vestiges.length > 0 && (
        <label className="block text-xs text-gray-600 dark:text-gray-400">
          {t("origin.vestige")}
          <select
            className={selectClass}
            value={value?.vestigeId ?? ""}
            onChange={(e) => value && onChange({ ...value, vestigeId: e.target.value ? Number(e.target.value) : undefined })}
          >
            <option value="">{t("origin.noVestige")}</option>
            {vestiges.map((v) => (
              <option key={v.id} value={v.id}>
                {v.label} · {v.description}
              </option>
            ))}
          </select>
        </label>
      )}
    </div>
  );
}
