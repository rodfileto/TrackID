import { useEffect, useState } from "react";
import { Link, useSearchParams } from "react-router";
import { useTranslation } from "react-i18next";
import PageMeta from "../components/common/PageMeta";
import PageBreadcrumb from "../components/common/PageBreadCrumb";
import ComponentCard from "../components/common/ComponentCard";
import {
  Table,
  TableBody,
  TableCell,
  TableHeader,
  TableRow,
} from "../components/ui/table";
import Badge from "../components/ui/badge/Badge";
import {
  listClusters,
  type ClusterSummary,
  type ListClustersResponse,
} from "../services/clusters";

const PAGE_SIZE = 25;

function modalityLabelKey(modality: string): string {
  return modality === "FACIAL" ? "modality.facial" : "modality.fingerprint";
}

function modalityColor(modality: string): "info" | "warning" {
  return modality === "FACIAL" ? "info" : "warning";
}

function formatDate(iso: string, locale: string): string {
  return new Date(iso).toLocaleDateString(locale);
}

function parsePage(value: string | null): number {
  const parsed = Number(value);
  return Number.isInteger(parsed) && parsed > 0 ? parsed : 1;
}

function IdentifiedCell({ cluster }: { cluster: ClusterSummary }) {
  if (!cluster.identified) {
    return <span className="text-gray-400 dark:text-white/30">—</span>;
  }
  return (
    <div className="flex flex-col gap-1">
      {(cluster.persons ?? []).map((person) => (
        <Link
          key={person.personId}
          to={`/persons/${encodeURIComponent(person.personId)}`}
          className="font-medium text-brand-500 hover:text-brand-600 dark:text-brand-400 dark:hover:text-brand-300"
        >
          {person.name}
        </Link>
      ))}
    </div>
  );
}

// Every persisted biometric cluster (cluster.Run's output) in one sortable,
// filterable list -- ordered by how many distinct criminal cases each one
// touches, so the clusters most relevant to active investigations surface
// first. See CaseClusters for the same data scoped to a single case.
export default function Clusters() {
  const { t, i18n } = useTranslation();
  const [searchParams, setSearchParams] = useSearchParams();

  const identifiedOnly = searchParams.get("identified") === "true";
  const page = parsePage(searchParams.get("page"));

  const [data, setData] = useState<ListClustersResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  function updateParams(patch: Record<string, string | undefined>) {
    setSearchParams((previous) => {
      const next = new URLSearchParams(previous);
      for (const [key, value] of Object.entries(patch)) {
        if (!value) {
          next.delete(key);
        } else {
          next.set(key, value);
        }
      }
      return next;
    });
  }

  useEffect(() => {
    let cancelled = false;

    async function load() {
      setLoading(true);
      setError("");
      try {
        const result = await listClusters({
          page,
          pageSize: PAGE_SIZE,
          identifiedOnly,
        });
        if (!cancelled) setData(result);
      } catch (err) {
        if (!cancelled) {
          setError(
            err instanceof Error ? err.message : t("clusters.loadError"),
          );
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    }

    load();
    return () => {
      cancelled = true;
    };
  }, [page, identifiedOnly, t]);

  function toggleIdentifiedOnly() {
    updateParams({
      identified: identifiedOnly ? undefined : "true",
      page: undefined,
    });
  }

  const items: ClusterSummary[] = data?.items ?? [];
  const total = data?.total ?? 0;
  const totalPages = data?.totalPages ?? 0;
  const currentPage = data?.page ?? page;

  return (
    <>
      <PageMeta
        title={`${t("clusters.title")} | TrackID`}
        description={t("clusters.metaDescription")}
      />
      <PageBreadcrumb pageTitle={t("clusters.title")} />

      <ComponentCard
        title={t("clusters.title")}
        desc={t("clusters.subtitle")}
      >
        <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <button
            type="button"
            onClick={toggleIdentifiedOnly}
            aria-pressed={identifiedOnly}
            className={`inline-flex h-11 items-center justify-center gap-2 rounded-lg border px-4 text-sm font-medium shadow-theme-xs transition-colors ${
              identifiedOnly
                ? "border-brand-500 bg-brand-500 text-white"
                : "border-gray-200 bg-white text-gray-700 hover:bg-gray-50 dark:border-gray-800 dark:bg-gray-900 dark:text-gray-400 dark:hover:bg-white/[0.03]"
            }`}
          >
            {t("clusters.identifiedOnly")}
          </button>

          <span className="text-sm text-gray-500 dark:text-gray-400">
            {t("clusters.count", { count: total })}
          </span>
        </div>

        <div className="overflow-hidden rounded-xl border border-gray-200 bg-white dark:border-white/[0.05] dark:bg-white/[0.03]">
          <div className="max-w-full overflow-x-auto">
            <Table>
              <TableHeader className="border-b border-gray-100 dark:border-white/[0.05]">
                <TableRow>
                  <TableCell
                    isHeader
                    className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
                  >
                    {t("clusters.columns.cluster")}
                  </TableCell>
                  <TableCell
                    isHeader
                    className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
                  >
                    {t("clusters.columns.type")}
                  </TableCell>
                  <TableCell
                    isHeader
                    className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
                  >
                    {t("clusters.columns.members")}
                  </TableCell>
                  <TableCell
                    isHeader
                    className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
                  >
                    {t("clusters.columns.cases")}
                  </TableCell>
                  <TableCell
                    isHeader
                    className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
                  >
                    {t("clusters.columns.identifiedAs")}
                  </TableCell>
                  <TableCell
                    isHeader
                    className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
                  >
                    {t("clusters.columns.created")}
                  </TableCell>
                </TableRow>
              </TableHeader>

              <TableBody className="divide-y divide-gray-100 dark:divide-white/[0.05]">
                {items.map((cluster) => (
                  <TableRow key={cluster.clusterId}>
                    <TableCell className="px-5 py-4 sm:px-6 text-start font-medium text-gray-800 dark:text-white/90">
                      #{cluster.clusterId}
                    </TableCell>
                    <TableCell className="px-4 py-3 text-start text-theme-sm">
                      <Badge size="sm" color={modalityColor(cluster.modality)}>
                        {t(modalityLabelKey(cluster.modality))}
                      </Badge>
                    </TableCell>
                    <TableCell className="px-4 py-3 text-gray-500 text-start text-theme-sm dark:text-gray-400">
                      {cluster.memberCount}
                    </TableCell>
                    <TableCell className="px-4 py-3 text-gray-500 text-start text-theme-sm dark:text-gray-400">
                      {cluster.caseCount}
                    </TableCell>
                    <TableCell className="px-4 py-3 text-start text-theme-sm">
                      <IdentifiedCell cluster={cluster} />
                    </TableCell>
                    <TableCell className="px-4 py-3 text-gray-500 text-start text-theme-sm dark:text-gray-400">
                      {formatDate(cluster.createdAt, i18n.resolvedLanguage ?? "en")}
                    </TableCell>
                  </TableRow>
                ))}

                {!loading && items.length === 0 && (
                  <TableRow>
                    <TableCell
                      colSpan={6}
                      className="px-5 py-8 text-center text-gray-500 dark:text-gray-400"
                    >
                      {error ? error : t("clusters.empty")}
                    </TableCell>
                  </TableRow>
                )}

                {loading && (
                  <TableRow>
                    <TableCell
                      colSpan={6}
                      className="px-5 py-8 text-center text-gray-500 dark:text-gray-400"
                    >
                      {t("common.loading")}
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          </div>
        </div>

        <div className="flex items-center justify-between">
          <span className="text-sm text-gray-500 dark:text-gray-400">
            {t("common.pageOf", { page: currentPage, total: totalPages || 1 })}
          </span>
          <div className="flex items-center gap-2">
            <button
              onClick={() =>
                updateParams({
                  page: String(Math.max(1, currentPage - 1)),
                })
              }
              disabled={currentPage <= 1}
              className="inline-flex h-9 items-center justify-center rounded-lg border border-gray-200 bg-white px-4 text-sm font-medium text-gray-700 shadow-theme-xs hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-gray-800 dark:bg-gray-900 dark:text-gray-400 dark:hover:bg-white/[0.03]"
            >
              {t("common.previous")}
            </button>
            <button
              onClick={() =>
                updateParams({
                  page: String(Math.min(totalPages || 1, currentPage + 1)),
                })
              }
              disabled={currentPage >= totalPages}
              className="inline-flex h-9 items-center justify-center rounded-lg border border-gray-200 bg-white px-4 text-sm font-medium text-gray-700 shadow-theme-xs hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-gray-800 dark:bg-gray-900 dark:text-gray-400 dark:hover:bg-white/[0.03]"
            >
              {t("common.next")}
            </button>
          </div>
        </div>
      </ComponentCard>
    </>
  );
}
