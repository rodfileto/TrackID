import { useEffect, useState, type FormEvent } from "react";
import { Link, useSearchParams } from "react-router";
import { toast } from "sonner";
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
import Button from "../components/ui/button/Button";
import { useModal } from "../hooks/useModal";
import CreateCaseModal from "../components/cases/CreateCaseModal";
import {
  listCases,
  listCaseYears,
  type Case,
  type ListCasesResponse,
} from "../services/cases";

const PAGE_SIZE = 10;

const FILTER_CASE_TYPES = [
  { value: "", label: "All types" },
  { value: "FACIAL", label: "Facial" },
  { value: "FINGERPRINT", label: "Fingerprint" },
];

function caseTypeLabel(caseType: string): string {
  return caseType === "FACIAL" ? "Facial" : "Fingerprint";
}

function caseTypeColor(caseType: string): "info" | "warning" {
  return caseType === "FACIAL" ? "info" : "warning";
}

function parsePage(value: string | null): number {
  const parsed = Number(value);
  return Number.isInteger(parsed) && parsed > 0 ? parsed : 1;
}

export default function CriminalCases() {
  const [searchParams, setSearchParams] = useSearchParams();

  const caseType = searchParams.get("caseType") ?? "";
  const year = searchParams.get("year") ?? "";
  const query = searchParams.get("q") ?? "";
  const page = parsePage(searchParams.get("page"));

  const [years, setYears] = useState<string[]>([]);
  const [search, setSearch] = useState(query);
  const [data, setData] = useState<ListCasesResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const [refreshKey, setRefreshKey] = useState(0);

  const { isOpen, openModal, closeModal } = useModal();

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
    listCaseYears()
      .then(setYears)
      .catch(() => setYears([]));
  }, [refreshKey]);

  useEffect(() => {
    let cancelled = false;

    async function load() {
      setLoading(true);
      setError("");
      try {
        const result = await listCases({
          page,
          pageSize: PAGE_SIZE,
          caseType: caseType || undefined,
          year: year || undefined,
          q: query || undefined,
        });
        if (!cancelled) setData(result);
      } catch (err) {
        if (!cancelled) {
          setError(
            err instanceof Error ? err.message : "Could not load cases",
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
  }, [page, caseType, year, query, refreshKey]);

  function handleCaseTypeChange(value: string) {
    updateParams({ caseType: value || undefined, page: undefined });
  }

  function handleYearChange(value: string) {
    updateParams({ year: value || undefined, page: undefined });
  }

  function handleSearchSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    updateParams({ q: search || undefined, page: undefined });
  }

  function handleCreated(created: Case) {
    toast.success(`Case ${created.caseId} created.`);
    updateParams({ page: undefined });
    setRefreshKey((key) => key + 1);
  }

  const items: Case[] = data?.items ?? [];
  const total = data?.total ?? 0;
  const totalPages = data?.totalPages ?? 0;
  const currentPage = data?.page ?? page;

  return (
    <>
      <PageMeta
        title="Criminal Cases | TrackID"
        description="List of criminal cases"
      />
      <PageBreadcrumb pageTitle="Criminal Cases" />

      <ComponentCard
        title="Criminal Cases"
        desc="Cases imported into TrackID, filterable by modality."
      >
        <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
            <select
              value={caseType}
              onChange={(event) => handleCaseTypeChange(event.target.value)}
              className="h-11 rounded-lg border border-gray-200 bg-transparent px-4 py-2.5 text-sm text-gray-800 shadow-theme-xs focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-800 dark:bg-gray-900 dark:text-white/90 dark:focus:border-brand-800"
            >
              {FILTER_CASE_TYPES.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </select>

            <select
              value={year}
              onChange={(event) => handleYearChange(event.target.value)}
              className="h-11 rounded-lg border border-gray-200 bg-transparent px-4 py-2.5 text-sm text-gray-800 shadow-theme-xs focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-800 dark:bg-gray-900 dark:text-white/90 dark:focus:border-brand-800"
            >
              <option value="">All years</option>
              {years.map((option) => (
                <option key={option} value={option}>
                  {option}
                </option>
              ))}
            </select>

            <form
              onSubmit={handleSearchSubmit}
              className="flex items-center gap-2"
            >
              <input
                type="text"
                value={search}
                onChange={(event) => setSearch(event.target.value)}
                placeholder="Search by description..."
                className="h-11 w-full rounded-lg border border-gray-200 bg-transparent px-4 py-2.5 text-sm text-gray-800 shadow-theme-xs placeholder:text-gray-400 focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-800 dark:bg-gray-900 dark:text-white/90 dark:placeholder:text-white/30 dark:focus:border-brand-800 sm:w-72"
              />
              <button
                type="submit"
                className="inline-flex h-11 items-center justify-center gap-2 rounded-lg border border-gray-200 bg-white px-4 py-2.5 text-sm font-medium text-gray-700 shadow-theme-xs hover:bg-gray-50 dark:border-gray-800 dark:bg-gray-900 dark:text-gray-400 dark:hover:bg-white/[0.03]"
              >
                Search
              </button>
            </form>
          </div>

          <div className="flex items-center gap-3">
            <span className="text-sm text-gray-500 dark:text-gray-400">
              {total} case{total === 1 ? "" : "s"}
            </span>
            <Button size="sm" onClick={openModal}>
              New Case
            </Button>
          </div>
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
                    Case ID
                  </TableCell>
                  <TableCell
                    isHeader
                    className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
                  >
                    Type
                  </TableCell>
                  <TableCell
                    isHeader
                    className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
                  >
                    Description
                  </TableCell>
                </TableRow>
              </TableHeader>

              <TableBody className="divide-y divide-gray-100 dark:divide-white/[0.05]">
                {items.map((item) => (
                  <TableRow key={item.caseId}>
                    <TableCell className="px-5 py-4 sm:px-6 text-start">
                      <Link
                        to={`/cases/${encodeURIComponent(item.caseId)}`}
                        state={{
                          from: searchParams.toString()
                            ? `/cases?${searchParams.toString()}`
                            : "/cases",
                        }}
                        className="block font-medium text-brand-500 hover:text-brand-600 dark:text-brand-400 dark:hover:text-brand-300"
                      >
                        {item.caseId}
                      </Link>
                    </TableCell>
                    <TableCell className="px-4 py-3 text-start text-theme-sm">
                      <Badge size="sm" color={caseTypeColor(item.caseType)}>
                        {caseTypeLabel(item.caseType)}
                      </Badge>
                    </TableCell>
                    <TableCell className="px-4 py-3 text-gray-500 text-start text-theme-sm dark:text-gray-400">
                      {item.description || "—"}
                    </TableCell>
                  </TableRow>
                ))}

                {!loading && items.length === 0 && (
                  <TableRow>
                    <TableCell className="px-5 py-8 text-center text-gray-500 dark:text-gray-400">
                      {error ? error : "No criminal cases found."}
                    </TableCell>
                  </TableRow>
                )}

                {loading && (
                  <TableRow>
                    <TableCell className="px-5 py-8 text-center text-gray-500 dark:text-gray-400">
                      Loading...
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          </div>
        </div>

        <div className="flex items-center justify-between">
          <span className="text-sm text-gray-500 dark:text-gray-400">
            Page {currentPage} of {totalPages || 1}
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
              Previous
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
              Next
            </button>
          </div>
        </div>
      </ComponentCard>

      <CreateCaseModal
        isOpen={isOpen}
        onClose={closeModal}
        onCreated={handleCreated}
      />
    </>
  );
}
