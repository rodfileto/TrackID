import { useEffect, useState, type FormEvent } from "react";
import { Link, useLocation, useNavigate, useSearchParams } from "react-router";
import { useTranslation } from "react-i18next";
import { useCurrentUser } from "../context/CurrentUserContext";
import { usePageMemory } from "../context/PageMemoryContext";
import PageMeta from "../components/common/PageMeta";
import PageBreadcrumb from "../components/common/PageBreadCrumb";
import ComponentCard from "../components/common/ComponentCard";
import Button from "../components/ui/button/Button";
import FileDropzone from "../components/common/FileDropzone";
import { IMAGE_FILES } from "../components/common/fileTypes";
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
  personPath,
  searchPersonsByFace,
  searchPersonsByName,
  type CaseFaceSearchResult,
  type ModalityCount,
  type PersonFaceSearchResult,
  type PersonSearchResult,
} from "../services/persons";

function modalityLabelKey(modality: string): string {
  return modality === "FACIAL" ? "modality.facial" : "modality.fingerprint";
}

function modalityColor(modality: string): "info" | "warning" {
  return modality === "FACIAL" ? "info" : "warning";
}

/** The "N facial / N fingerprint" badges next to a person result -- how many
 * criminal cases (see PersonSearchResult.caseCounts) that person is linked
 * to, broken down by case type. Renders nothing when the person has none -- and for anyone but
 * an analyst, whom the server sends no counts. */
function CaseCountBadges({ caseCounts }: { caseCounts: ModalityCount[] | null }) {
  const { t } = useTranslation();
  if (!caseCounts || caseCounts.length === 0) {
    return <span className="text-gray-400 dark:text-white/30">—</span>;
  }
  return (
    <div className="flex flex-wrap gap-1">
      {caseCounts.map((c) => (
        <Badge key={c.modality} size="sm" color={modalityColor(c.modality)}>
          {c.count} {t(modalityLabelKey(c.modality))}
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
                    to={personPath(result.personNumber)}
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
                  <Badge size="sm" color={modalityColor(result.modality)}>
                    {t(modalityLabelKey(result.modality))}
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

interface PersonLookupState {
  name?: string;
}

function NameSearch() {
  const { t } = useTranslation();
  const location = useLocation();
  const navigate = useNavigate();
  // The searched name is someone's name, so it stays out of the URL, and with it the browser's
  // history, its address-bar suggestions and any proxy's logs. Kept on the history entry instead,
  // back, forward and reload still bring the results back.
  const query = (location.state as PersonLookupState | null)?.name ?? "";
  const [name, setName] = useState(query);
  const [shownQuery, setShownQuery] = useState(query);
  if (query !== shownQuery) {
    setShownQuery(query);
    setName(query);
  }
  const [found, setFound] = useState<{ query: string; results: PersonSearchResult[]; error: string } | null>(null);
  const loading = query !== "" && found?.query !== query;

  useEffect(() => {
    if (!query) return;
    let cancelled = false;
    searchPersonsByName(query)
      .then((results) => {
        if (!cancelled) setFound({ query, results, error: "" });
      })
      .catch((err) => {
        if (!cancelled) setFound({ query, results: [], error: err instanceof Error ? err.message : t("personLookup.searchError") });
      });
    return () => {
      cancelled = true;
    };
  }, [query, t]);

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const trimmed = name.trim();
    if (!trimmed) return;
    navigate(location, { state: { name: trimmed } satisfies PersonLookupState });
  }

  const current = found?.query === query ? found : null;

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
          autoFocus={!query}
          className="h-11 w-full rounded-lg border border-gray-200 bg-transparent px-4 py-2.5 text-sm text-gray-800 shadow-theme-xs placeholder:text-gray-400 focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-800 dark:bg-gray-900 dark:text-white/90 dark:placeholder:text-white/30 dark:focus:border-brand-800 sm:w-80"
        />
        <Button size="sm" disabled={!name.trim() || loading}>
          {loading ? t("personLookup.searching") : t("common.search")}
        </Button>
      </form>

      {current?.error && <p className="text-sm text-error-500">{current.error}</p>}

      {current && !current.error && (
        <ResultsTable
          results={current.results}
          showSimilarity={false}
          emptyMessage={t("personLookup.noPersonsForName", { name: current.query })}
        />
      )}
    </>
  );
}

/** A face search's photo and matches, which cannot go on a history entry. */
interface FaceSearchMemory {
  file: File | null;
  searched: boolean;
  personResults: PersonFaceSearchResult[];
  caseResults: CaseFaceSearchResult[];
}

function FaceSearch() {
  const { t } = useTranslation();
  const memory = usePageMemory<FaceSearchMemory>("personLookup.faceSearch");
  const [remembered] = useState(() => memory.read());
  const [file, setFile] = useState<File | null>(remembered?.file ?? null);
  const [previewUrl, setPreviewUrl] = useState<string | null>(null);
  const [searched, setSearched] = useState(remembered?.searched ?? false);
  const [personResults, setPersonResults] = useState<PersonFaceSearchResult[]>(
    remembered?.personResults ?? [],
  );
  const [caseResults, setCaseResults] = useState<CaseFaceSearchResult[]>(remembered?.caseResults ?? []);
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

  useEffect(() => {
    memory.write({ file, searched, personResults, caseResults });
  }, [memory, file, searched, personResults, caseResults]);

  function chooseFile(chosen: File) {
    setFile(chosen);
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
          <FileDropzone
            variant="compact"
            className="sm:w-80"
            accept={IMAGE_FILES}
            label={file?.name}
            onFiles={([chosen]) => chooseFile(chosen)}
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
// before opening their page.
export default function PersonLookup() {
  const { t } = useTranslation();
  const location = useLocation();
  const [searchParams, setSearchParams] = useSearchParams();
  // Face search returns cases and clusters: analysts only.
  const { isIntelligence } = useCurrentUser();
  const mode: SearchMode = searchParams.get("mode") === "face" && isIntelligence ? "face" : "name";
  const setMode = (next: SearchMode) =>
    setSearchParams(
      (previous) => {
        const params = new URLSearchParams(previous);
        if (next === "face") params.set("mode", "face");
        else params.delete("mode");
        return params;
      },
      { replace: true, state: location.state },
    );

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
          {tabs.filter((tab) => tab.mode === "name" || isIntelligence).map((tab) => (
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
