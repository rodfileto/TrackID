import { useEffect, useState, type FormEvent } from "react";
import { Link } from "react-router";
import { useTranslation } from "react-i18next";
import PageMeta from "../components/common/PageMeta";
import PageBreadcrumb from "../components/common/PageBreadCrumb";
import ComponentCard from "../components/common/ComponentCard";
import Button from "../components/ui/button/Button";
import FileInput from "../components/form/input/FileInput";
import {
  Table,
  TableBody,
  TableCell,
  TableHeader,
  TableRow,
} from "../components/ui/table";
import Badge from "../components/ui/badge/Badge";
import IdentityFileThumbnail from "../components/persons/IdentityFileThumbnail";
import CaseTraceThumbnail from "../components/cases/CaseTraceThumbnail";
import {
  searchPersonsByFace,
  searchPersonsByName,
  type CaseFaceSearchResult,
  type CaseTypeCount,
  type PersonFaceSearchResult,
  type PersonSearchResult,
} from "../services/persons";

function caseTypeLabelKey(caseType: string): string {
  return caseType === "FACIAL" ? "caseType.facial" : "caseType.fingerprint";
}

function caseTypeColor(caseType: string): "info" | "warning" {
  return caseType === "FACIAL" ? "info" : "warning";
}

/** The "N facial / N fingerprint" badges next to a person result -- how many
 * criminal cases (see PersonSearchResult.caseCounts) that person is linked
 * to, broken down by case type. Renders nothing when the person has none. */
function CaseCountBadges({ caseCounts }: { caseCounts: CaseTypeCount[] | null }) {
  const { t } = useTranslation();
  if (!caseCounts || caseCounts.length === 0) {
    return <span className="text-gray-400 dark:text-white/30">—</span>;
  }
  return (
    <div className="flex flex-wrap gap-1">
      {caseCounts.map((c) => (
        <Badge key={c.caseType} size="sm" color={caseTypeColor(c.caseType)}>
          {c.count} {t(caseTypeLabelKey(c.caseType))}
        </Badge>
      ))}
    </div>
  );
}

type SearchMode = "name" | "face";

const tabs: { mode: SearchMode; labelKey: string }[] = [
  { mode: "name", labelKey: "personLookup.byName" },
  { mode: "face", labelKey: "personLookup.byFace" },
];

function tabButtonClasses(active: boolean): string {
  return `px-4 py-2 text-sm font-medium rounded-lg transition-colors ${
    active
      ? "bg-brand-500 text-white"
      : "bg-transparent text-gray-500 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-white/[0.05]"
  }`;
}

/** The columns every results table shares -- similarity and a match photo
 * are extra columns only the face search variant renders. */
function ResultsTable({
  results,
  emptyMessage,
  showSimilarity,
}: {
  results: PersonSearchResult[] | PersonFaceSearchResult[];
  emptyMessage: string;
  showSimilarity: boolean;
}) {
  const { t } = useTranslation();
  return (
    <div className="overflow-hidden rounded-xl border border-gray-200 bg-white dark:border-white/[0.05] dark:bg-white/[0.03]">
      <div className="max-w-full overflow-x-auto">
        <Table>
          <TableHeader className="border-b border-gray-100 dark:border-white/[0.05]">
            <TableRow>
              {showSimilarity && (
                <TableCell
                  isHeader
                  className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
                >
                  {t("personLookup.columns.photo")}
                </TableCell>
              )}
              <TableCell
                isHeader
                className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
              >
                {t("personLookup.columns.name")}
              </TableCell>
              <TableCell
                isHeader
                className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
              >
                {t("personLookup.columns.document")}
              </TableCell>
              <TableCell
                isHeader
                className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
              >
                {t("personLookup.columns.register")}
              </TableCell>
              <TableCell
                isHeader
                className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
              >
                {t("personLookup.columns.cases")}
              </TableCell>
              {showSimilarity && (
                <TableCell
                  isHeader
                  className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
                >
                  {t("personLookup.columns.similarity")}
                </TableCell>
              )}
            </TableRow>
          </TableHeader>
          <TableBody className="divide-y divide-gray-100 dark:divide-white/[0.05]">
            {results.map((result, index) => (
              <TableRow key={`${result.personId}-${index}`}>
                {showSimilarity && (
                  <TableCell className="px-5 py-4 sm:px-6 text-start">
                    <IdentityFileThumbnail
                      personId={result.personId}
                      identityFileId={
                        (result as PersonFaceSearchResult).identityFileId
                      }
                      contentType={
                        (result as PersonFaceSearchResult).contentType
                      }
                      alt={result.name}
                    />
                  </TableCell>
                )}
                <TableCell className="px-5 py-4 sm:px-6 text-start">
                  <Link
                    to={`/persons/${encodeURIComponent(result.personId)}`}
                    className="block font-medium text-brand-500 hover:text-brand-600 dark:text-brand-400 dark:hover:text-brand-300"
                  >
                    {result.name}
                  </Link>
                </TableCell>
                <TableCell className="px-4 py-3 text-gray-500 text-start text-theme-sm dark:text-gray-400">
                  {result.documentType} {result.documentNumber}
                </TableCell>
                <TableCell className="px-4 py-3 text-gray-500 text-start text-theme-sm dark:text-gray-400">
                  {result.registerNumber}
                </TableCell>
                <TableCell className="px-4 py-3 text-start text-theme-sm">
                  <CaseCountBadges caseCounts={result.caseCounts} />
                </TableCell>
                {showSimilarity && (
                  <TableCell className="px-4 py-3 text-gray-500 text-start text-theme-sm dark:text-gray-400">
                    {((result as PersonFaceSearchResult).similarity * 100).toFixed(1)}%
                  </TableCell>
                )}
              </TableRow>
            ))}

            {results.length === 0 && (
              <TableRow>
                <TableCell
                  colSpan={showSimilarity ? 6 : 4}
                  className="px-5 py-8 text-center text-gray-500 dark:text-gray-400"
                >
                  {emptyMessage}
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </div>
    </div>
  );
}

/** The "Matching criminal cases" table a face search's QUESTIONED matches
 * render into, below the enrolled-person results -- similarity here isn't
 * comparable to a person result's (KNOWN vs QUESTIONED embeddings), so this
 * stays a separate table rather than a merged ranked list. */
function CaseResultsTable({ results }: { results: CaseFaceSearchResult[] }) {
  const { t } = useTranslation();
  return (
    <div className="overflow-hidden rounded-xl border border-gray-200 bg-white dark:border-white/[0.05] dark:bg-white/[0.03]">
      <div className="max-w-full overflow-x-auto">
        <Table>
          <TableHeader className="border-b border-gray-100 dark:border-white/[0.05]">
            <TableRow>
              <TableCell
                isHeader
                className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
              >
                {t("personLookup.columns.photo")}
              </TableCell>
              <TableCell
                isHeader
                className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
              >
                {t("personLookup.columns.caseId")}
              </TableCell>
              <TableCell
                isHeader
                className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
              >
                {t("personLookup.columns.type")}
              </TableCell>
              <TableCell
                isHeader
                className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
              >
                {t("personLookup.columns.description")}
              </TableCell>
              <TableCell
                isHeader
                className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
              >
                {t("personLookup.columns.similarity")}
              </TableCell>
            </TableRow>
          </TableHeader>
          <TableBody className="divide-y divide-gray-100 dark:divide-white/[0.05]">
            {results.map((result) => (
              <TableRow key={`${result.caseId}-${result.traceId}`}>
                <TableCell className="px-5 py-4 sm:px-6 text-start">
                  <CaseTraceThumbnail
                    caseId={result.caseId}
                    thumbnailFileId={result.thumbnailFileId}
                    thumbnailBox={result.thumbnailBox}
                    alt={t("personLookup.traceAlt", { traceId: result.traceId, caseId: result.caseId })}
                  />
                </TableCell>
                <TableCell className="px-5 py-4 sm:px-6 text-start">
                  <Link
                    to={`/cases/${encodeURIComponent(result.caseId)}`}
                    className="block font-medium text-brand-500 hover:text-brand-600 dark:text-brand-400 dark:hover:text-brand-300"
                  >
                    {result.caseId}
                  </Link>
                </TableCell>
                <TableCell className="px-4 py-3 text-start text-theme-sm">
                  <Badge size="sm" color={caseTypeColor(result.caseType)}>
                    {t(caseTypeLabelKey(result.caseType))}
                  </Badge>
                </TableCell>
                <TableCell className="px-4 py-3 text-gray-500 text-start text-theme-sm dark:text-gray-400">
                  {result.description || "—"}
                </TableCell>
                <TableCell className="px-4 py-3 text-gray-500 text-start text-theme-sm dark:text-gray-400">
                  {(result.similarity * 100).toFixed(1)}%
                </TableCell>
              </TableRow>
            ))}

            {results.length === 0 && (
              <TableRow>
                <TableCell
                  colSpan={5}
                  className="px-5 py-8 text-center text-gray-500 dark:text-gray-400"
                >
                  {t("personLookup.noCasesForFace")}
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </div>
    </div>
  );
}

function NameSearch() {
  const { t } = useTranslation();
  const [name, setName] = useState("");
  const [searchedFor, setSearchedFor] = useState<string | null>(null);
  const [results, setResults] = useState<PersonSearchResult[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const trimmed = name.trim();
    if (!trimmed) return;

    setLoading(true);
    setError("");
    try {
      setResults(await searchPersonsByName(trimmed));
      setSearchedFor(trimmed);
    } catch (err) {
      setError(
        err instanceof Error ? err.message : t("personLookup.searchError"),
      );
      setResults([]);
      setSearchedFor(trimmed);
    } finally {
      setLoading(false);
    }
  }

  return (
    <>
      <form
        onSubmit={handleSubmit}
        className="flex flex-col gap-3 sm:flex-row sm:items-center"
      >
        <input
          type="text"
          value={name}
          onChange={(event) => setName(event.target.value)}
          placeholder={t("personLookup.namePlaceholder")}
          autoFocus
          className="h-11 w-full rounded-lg border border-gray-200 bg-transparent px-4 py-2.5 text-sm text-gray-800 shadow-theme-xs placeholder:text-gray-400 focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-800 dark:bg-gray-900 dark:text-white/90 dark:placeholder:text-white/30 dark:focus:border-brand-800 sm:w-80"
        />
        <Button size="sm" disabled={!name.trim() || loading}>
          {loading ? t("personLookup.searching") : t("common.search")}
        </Button>
      </form>

      {error && <p className="text-sm text-error-500">{error}</p>}

      {searchedFor !== null && !error && (
        <ResultsTable
          results={results}
          showSimilarity={false}
          emptyMessage={t("personLookup.noPersonsForName", { name: searchedFor })}
        />
      )}
    </>
  );
}

function FaceSearch() {
  const { t } = useTranslation();
  const [file, setFile] = useState<File | null>(null);
  const [previewUrl, setPreviewUrl] = useState<string | null>(null);
  const [searched, setSearched] = useState(false);
  const [personResults, setPersonResults] = useState<PersonFaceSearchResult[]>(
    [],
  );
  const [caseResults, setCaseResults] = useState<CaseFaceSearchResult[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!file) {
      setPreviewUrl(null);
      return;
    }
    const url = URL.createObjectURL(file);
    setPreviewUrl(url);
    return () => URL.revokeObjectURL(url);
  }, [file]);

  function handleFileChange(event: React.ChangeEvent<HTMLInputElement>) {
    setFile(event.target.files?.[0] ?? null);
    setSearched(false);
    setError("");
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!file) return;

    setLoading(true);
    setError("");
    try {
      const { persons, cases } = await searchPersonsByFace(file);
      setPersonResults(persons);
      setCaseResults(cases);
      setSearched(true);
    } catch (err) {
      setError(
        err instanceof Error ? err.message : t("personLookup.searchError"),
      );
      setPersonResults([]);
      setCaseResults([]);
      setSearched(true);
    } finally {
      setLoading(false);
    }
  }

  return (
    <>
      <form onSubmit={handleSubmit} className="flex flex-col gap-3">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
          <FileInput
            className="sm:w-80"
            onChange={(event) => handleFileChange(event)}
          />
          <Button size="sm" disabled={!file || loading}>
            {loading ? t("personLookup.searching") : t("common.search")}
          </Button>
        </div>
        {previewUrl && (
          <img
            src={previewUrl}
            alt={t("personLookup.previewAlt")}
            className="h-32 w-32 rounded-lg border border-gray-200 object-cover dark:border-white/[0.05]"
          />
        )}
      </form>

      {error && <p className="text-sm text-error-500">{error}</p>}

      {searched && !error && (
        <>
          <ResultsTable
            results={personResults}
            showSimilarity
            emptyMessage={t("personLookup.noPersonsForFace")}
          />

          <div className="flex flex-col gap-3">
            <h4 className="text-base font-medium text-gray-800 dark:text-white/90">
              {t("personLookup.matchingCases")}
            </h4>
            <CaseResultsTable results={caseResults} />
          </div>
        </>
      )}
    </>
  );
}

// There is no list-persons endpoint (persons are enrolled through an
// organization's own import command, not created here) -- this page finds
// a person by name (see person.SearchByName) or by uploading a photo of a
// face (see person.SearchByFace), for an analyst to pick the right match
// before opening /persons/:personId.
export default function PersonLookup() {
  const { t } = useTranslation();
  const [mode, setMode] = useState<SearchMode>("name");

  return (
    <>
      <PageMeta
        title={`${t("personLookup.pageTitle")} | TrackID`}
        description={t("personLookup.metaDescription")}
      />
      <PageBreadcrumb pageTitle={t("personLookup.pageTitle")} />

      <ComponentCard
        title={t("personLookup.title")}
        desc={t("personLookup.subtitle")}
      >
        <div className="flex gap-2">
          {tabs.map((tab) => (
            <button
              key={tab.mode}
              type="button"
              onClick={() => setMode(tab.mode)}
              className={tabButtonClasses(mode === tab.mode)}
            >
              {t(tab.labelKey)}
            </button>
          ))}
        </div>

        {mode === "name" ? <NameSearch /> : <FaceSearch />}
      </ComponentCard>
    </>
  );
}
