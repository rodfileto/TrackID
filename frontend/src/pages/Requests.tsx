import { useEffect, useState } from "react";
import { Link } from "react-router";
import { useTranslation } from "react-i18next";
import PageMeta from "../components/common/PageMeta";
import PageBreadcrumb from "../components/common/PageBreadCrumb";
import ComponentCard from "../components/common/ComponentCard";
import Badge from "../components/ui/badge/Badge";
import { citedNumbers, formatReferences, listRequests, type RequestSummary } from "../services/requests";
import { typeKey } from "../components/analyses/labels";
import type { AnalysisType } from "../services/analyses";

function dateBR(iso?: string): string {
  if (!iso) return "";
  const [y, m, d] = iso.slice(0, 10).split("-");
  return `${d}/${m}/${y}`;
}

/** Coordinator: every request received, soonest due first, with the cases it opened. */
export default function Requests() {
  const { t } = useTranslation();
  const [items, setItems] = useState<RequestSummary[] | null>(null);
  const [error, setError] = useState("");
  const [search, setSearch] = useState("");
  const [reference, setReference] = useState("");

  useEffect(() => {
    listRequests(reference)
      .then(setItems)
      .catch((err) => setError(err instanceof Error ? err.message : String(err)));
  }, [reference]);

  return (
    <>
      <PageMeta title={`${t("requests.navTitle")} | TrackID`} description={t("requests.listDesc")} />
      <PageBreadcrumb pageTitle={t("requests.navTitle")} />
      <ComponentCard title={t("requests.navTitle")} desc={t("requests.listDesc")}>
        <div>
          <Link
            to="/requests/new"
            className="inline-flex h-10 items-center rounded-lg bg-brand-500 px-4 text-sm font-medium text-white hover:bg-brand-600"
          >
            {t("requests.new")}
          </Link>
        </div>
        <form
          className="flex max-w-md gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            setReference(search.trim());
          }}
        >
          <input
            aria-label={t("requests.searchReference")}
            placeholder={t("requests.searchReference")}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="h-10 min-w-0 flex-1 rounded-lg border border-gray-300 bg-transparent px-3 text-sm text-gray-800 dark:border-gray-700 dark:text-white/90"
          />
          <button type="submit" className="h-10 rounded-lg border border-gray-300 px-4 text-sm text-gray-700 hover:bg-gray-50 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-white/5">
            {t("requests.search")}
          </button>
        </form>
        {error && <p className="text-sm text-error-500">{error}</p>}
        {items && items.length === 0 && <p className="text-sm text-gray-500 dark:text-gray-400">{t("requests.none")}</p>}
        {items && items.length > 0 && (
          <div className="overflow-x-auto">
            <table className="min-w-full text-left text-sm">
              <thead className="border-b border-gray-200 text-theme-xs uppercase text-gray-500 dark:border-gray-800 dark:text-gray-400">
                <tr>
                  {(["document", "authority", "received", "due", "occurrence", "cases", "tasks", "open"] as const).map((c) => (
                    <th key={c} className="px-3 py-2 font-medium">
                      {t(`requests.columns.${c}`)}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100 text-gray-700 dark:divide-gray-800 dark:text-gray-300">
                {items.map((r) => (
                  <tr key={r.id}>
                    <td className="px-3 py-2 font-medium text-gray-800 dark:text-white/90">
                      {r.documentReference}
                      {r.references.length > 0 && (
                        <span className="block text-theme-xs font-normal text-gray-500 dark:text-gray-400">{formatReferences(r.references)}</span>
                      )}
                    </td>
                    <td className="px-3 py-2">{r.authority}</td>
                    <td className="px-3 py-2">{dateBR(r.receivedOn)}</td>
                    <td className="px-3 py-2">
                      {dateBR(r.dueOn)}{" "}
                      {r.overdue && (
                        <Badge size="sm" color="error">
                          {t("requests.overdue")}
                        </Badge>
                      )}
                    </td>
                    <td className="px-3 py-2">{r.occurrenceNumber ?? (r.citedReference || r.citedInquiryNumber ? t("requests.occurrenceUnlinked", { reference: citedNumbers(r) }) : "—")}</td>
                    <td className="px-3 py-2">
                      {r.caseIds.map((id, i) => (
                        <span key={id}>
                          {i > 0 && ", "}
                          <Link to={`/cases/${encodeURIComponent(id)}`} className="text-brand-500 hover:underline">
                            {id}
                          </Link>
                        </span>
                      ))}
                    </td>
                    <td className="px-3 py-2">{r.tasks.map((task) => t(typeKey(task as AnalysisType))).join(", ") || "—"}</td>
                    <td className="px-3 py-2">{r.openAnalyses}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </ComponentCard>
    </>
  );
}
