import { useEffect, useState } from "react";
import { Link } from "react-router";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import ComponentCard from "../common/ComponentCard";
import OccurrencePointMap from "../intelligence/OccurrencePointMap";
import {
  getCaseOccurrence,
  linkCaseOccurrence,
  searchOccurrences,
  type OccurrenceSummary,
} from "../../services/occurrences";

function label(o: OccurrenceSummary): string {
  const police = o.agency && o.number ? `${o.agency} · ${o.number}` : "";
  return [o.trackidNumber, police].filter(Boolean).join(" — ");
}

// The occurrence a case investigates: shown with its point on a small map and a link to it on
// the occurrence map, or
// searched by TrackID or police number and linked. The API refuses a link
// between a criminal case and a civil occurrence, or the reverse.
export default function CaseOccurrenceCard({ caseId }: { caseId: string }) {
  const { t, i18n } = useTranslation();
  const [linked, setLinked] = useState<OccurrenceSummary | null>(null);
  const [loading, setLoading] = useState(true);
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<OccurrenceSummary[] | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    getCaseOccurrence(caseId)
      .then((o) => !cancelled && setLinked(o))
      .catch(() => !cancelled && setLinked(null))
      .finally(() => !cancelled && setLoading(false));
    return () => {
      cancelled = true;
    };
  }, [caseId]);

  async function search(e: React.FormEvent) {
    e.preventDefault();
    try {
      setResults(await searchOccurrences(query.trim()));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("caseOccurrence.searchError"));
    }
  }

  async function link(o: OccurrenceSummary | null) {
    setBusy(true);
    try {
      await linkCaseOccurrence(caseId, o?.id ?? null);
      // A search result has no location point: read the linked occurrence back for the map.
      setLinked(o ? ((await getCaseOccurrence(caseId)) ?? o) : null);
      setResults(null);
      setQuery("");
      toast.success(t(o ? "caseOccurrence.linkedToast" : "caseOccurrence.unlinkedToast"));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("caseOccurrence.linkError"));
    } finally {
      setBusy(false);
    }
  }

  const describe = (o: OccurrenceSummary) =>
    [
      o.types.map((c) => t(`occurrenceType.${c}`)).join(" · "),
      new Date(o.occurredFrom).toLocaleDateString(i18n.language),
      o.placeName || o.neighbourhood,
    ]
      .filter(Boolean)
      .join(" — ");

  return (
    <ComponentCard title={t("caseOccurrence.title")} desc={t("caseOccurrence.desc")}>
      {loading ? (
        <p className="text-sm text-gray-500 dark:text-gray-400">{t("common.loading")}</p>
      ) : linked ? (
        <div className="flex flex-col gap-4">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div className="flex flex-col gap-0.5">
              <Link
                to={`/intelligence/map?year=${new Date(linked.occurredFrom).getFullYear()}&occurrence=${linked.id}`}
                className="font-medium text-brand-500 hover:text-brand-600 dark:text-brand-400"
              >
                {label(linked)}
              </Link>
              <span className="text-sm text-gray-500 dark:text-gray-400">{describe(linked)}</span>
              {linked.address && (
                <span className="text-sm text-gray-500 dark:text-gray-400">{linked.address}</span>
              )}
            </div>
            <button
              type="button"
              disabled={busy}
              onClick={() => link(null)}
              className="h-10 shrink-0 rounded-lg border border-gray-300 px-4 text-sm font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-50 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-white/[0.03]"
            >
              {t("caseOccurrence.unlink")}
            </button>
          </div>
          {linked.lon !== undefined && linked.lat !== undefined && (
            <OccurrencePointMap lon={linked.lon} lat={linked.lat} precision={linked.precision} />
          )}
        </div>
      ) : (
        <div className="flex flex-col gap-3">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <p className="text-sm text-gray-500 dark:text-gray-400">{t("caseOccurrence.none")}</p>
            <Link
              to={`/intelligence/occurrences/new?case=${encodeURIComponent(caseId)}`}
              className="inline-flex h-10 shrink-0 items-center rounded-lg bg-brand-500 px-4 text-sm font-medium text-white hover:bg-brand-600"
            >
              {t("caseOccurrence.registerNew")}
            </Link>
          </div>
          <form onSubmit={search} className="flex gap-2">
            <input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder={t("caseOccurrence.searchPlaceholder")}
              aria-label={t("caseOccurrence.searchPlaceholder")}
              className="h-11 w-full rounded-lg border border-gray-300 bg-transparent px-4 text-sm text-gray-800 shadow-theme-xs placeholder:text-gray-400 focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90"
            />
            <button
              type="submit"
              disabled={query.trim().length < 2}
              className="h-11 shrink-0 rounded-lg bg-brand-500 px-4 text-sm font-medium text-white hover:bg-brand-600 disabled:opacity-50"
            >
              {t("occurrenceForm.search")}
            </button>
          </form>
          {results && (
            <ul className="flex flex-col divide-y divide-gray-100 rounded-lg border border-gray-200 dark:divide-gray-800 dark:border-gray-800">
              {results.length === 0 && (
                <li className="px-3 py-2 text-sm text-gray-500 dark:text-gray-400">{t("occurrenceForm.noResults")}</li>
              )}
              {results.map((o) => (
                <li key={o.id} className="flex items-center justify-between gap-3 px-3 py-2">
                  <div className="flex flex-col">
                    <span className="text-sm font-medium text-gray-800 dark:text-white/90">{label(o)}</span>
                    <span className="text-theme-xs text-gray-500 dark:text-gray-400">{describe(o)}</span>
                  </div>
                  <button
                    type="button"
                    disabled={busy}
                    onClick={() => link(o)}
                    className="h-9 shrink-0 rounded-lg border border-brand-500 px-3 text-sm font-medium text-brand-500 hover:bg-brand-50 disabled:opacity-50 dark:hover:bg-brand-500/10"
                  >
                    {t("caseOccurrence.link")}
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </ComponentCard>
  );
}
