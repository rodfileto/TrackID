import { useEffect, useState, type FormEvent } from "react";
import { useLocation, useNavigate, useSearchParams } from "react-router";
import { useTranslation } from "react-i18next";
import { useCurrentUser } from "../context/CurrentUserContext";
import { usePageMemory } from "../context/PageMemoryContext";
import PageMeta from "../components/common/PageMeta";
import PageBreadcrumb from "../components/common/PageBreadCrumb";
import ComponentCard from "../components/common/ComponentCard";
import Button from "../components/ui/button/Button";
import FileDropzone from "../components/common/FileDropzone";
import { IMAGE_FILES } from "../components/common/fileTypes";
import { CaseResultsTable, ResultsTable } from "../components/persons/FaceSearchResults";
import {
  searchPersonsByFace,
  searchPersonsByName,
  type CaseFaceSearchResult,
  type PersonFaceSearchResult,
  type PersonSearchResult,
} from "../services/persons";

type SearchMode = "name" | "face";

interface PersonLookupState {
  name?: string;
}

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
