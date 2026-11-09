import { useCallback, useEffect, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { Modal } from "../ui/modal";
import Button from "../ui/button/Button";
import Spinner from "../ui/spinner/Spinner";
import { CaseResultsTable, ResultsTable } from "../persons/FaceSearchResults";
import { searchFromTrace, type TraceFaceSearchResults } from "../../services/persons";

/** Intelligence: searches the enrolled persons and the other cases' faces from one of the case's faces, at a
 * threshold the analyst adjusts. It shows what resembles the face at that score and decides nothing: the automatic
 * matcher keeps its own threshold. Each search is logged as a face search on the case. */
export default function TraceFaceSearchModal({
  caseId,
  traceId,
  label,
  onClose,
}: {
  caseId: string;
  traceId: number;
  label: string;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [threshold, setThreshold] = useState("");
  const [results, setResults] = useState<TraceFaceSearchResults | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const run = useCallback(
    async (minSimilarity?: number) => {
      setLoading(true);
      setError("");
      try {
        const found = await searchFromTrace(caseId, traceId, minSimilarity);
        setResults(found);
        setThreshold(found.minSimilarity.toFixed(2));
      } catch (err) {
        setError(err instanceof Error ? err.message : t("traceSearch.error"));
      } finally {
        setLoading(false);
      }
    },
    [caseId, traceId, t],
  );

  // The first search runs at the matcher's own review threshold, which the answer says.
  useEffect(() => {
    void run();
  }, [run]);

  const value = Number(threshold);
  const valid = threshold !== "" && value >= 0 && value <= 1;

  function submit(event: FormEvent) {
    event.preventDefault();
    if (valid) void run(value);
  }

  return (
    <Modal isOpen onClose={onClose} className="m-4 max-w-5xl p-6">
      <h3 className="text-lg font-semibold text-gray-800 dark:text-white/90">{t("traceSearch.title", { face: label })}</h3>
      <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">{t("traceSearch.lead")}</p>

      <form onSubmit={submit} className="mt-4 flex flex-wrap items-end gap-4">
        <label className="flex flex-col gap-1 text-sm text-gray-700 dark:text-gray-300">
          {t("traceSearch.threshold")}
          <span className="flex items-center gap-3">
            <input
              type="range"
              min={0.2}
              max={0.9}
              step={0.01}
              value={valid ? value : 0.4}
              onChange={(e) => setThreshold(Number(e.target.value).toFixed(2))}
              className="w-56 accent-brand-500"
              aria-label={t("traceSearch.threshold")}
            />
            <input
              type="number"
              min={0}
              max={1}
              step={0.01}
              value={threshold}
              onChange={(e) => setThreshold(e.target.value)}
              className="h-9 w-20 rounded-lg border border-gray-300 bg-transparent px-2 text-sm text-gray-800 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90"
            />
          </span>
        </label>
        <Button size="sm" type="submit" disabled={loading || !valid} startIcon={loading ? <Spinner /> : undefined}>
          {loading ? t("traceSearch.searching") : t("traceSearch.search")}
        </Button>
        {results && (
          <span className="text-theme-xs text-gray-500 dark:text-gray-400">
            {t("traceSearch.matcher", { threshold: results.matcherThreshold.toFixed(2) })}
          </span>
        )}
      </form>

      {error && <p className="mt-4 text-sm text-error-500">{error}</p>}

      {results && (
        <div className="mt-6 max-h-[60vh] space-y-6 overflow-y-auto">
          <div>
            <h4 className="mb-2 text-sm font-medium text-gray-800 dark:text-white/90">
              {t("traceSearch.persons", { count: results.persons.length, threshold: results.minSimilarity.toFixed(2) })}
            </h4>
            <ResultsTable results={results.persons} emptyMessage={t("traceSearch.noPersons")} showSimilarity />
          </div>
          <div>
            <h4 className="mb-2 text-sm font-medium text-gray-800 dark:text-white/90">
              {t("traceSearch.cases", { count: results.cases.length })}
            </h4>
            {results.cases.length === 0 ? (
              <p className="text-sm text-gray-500 dark:text-gray-400">{t("traceSearch.noCases")}</p>
            ) : (
              <CaseResultsTable results={results.cases} />
            )}
          </div>
        </div>
      )}
    </Modal>
  );
}
