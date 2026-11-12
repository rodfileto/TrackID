import { useEffect, useMemo, useState } from "react";
import { Link, useSearchParams } from "react-router";
import { useTranslation } from "react-i18next";
import PageMeta from "../components/common/PageMeta";
import PageBreadcrumb from "../components/common/PageBreadCrumb";
import ComponentCard from "../components/common/ComponentCard";
import OccurrenceMapView, {
  type MapMode,
} from "../components/intelligence/OccurrenceMapView";
import OccurrenceDetails from "../components/intelligence/OccurrenceDetails";
import { GROUP_COLORS } from "../components/intelligence/groupColors";
import {
  getOccurrenceMap,
  listOccurrenceYears,
  type OccurrenceFeature,
  type OccurrenceMap as OccurrenceMapData,
} from "../services/occurrences";

const GROUPS = Object.keys(GROUP_COLORS);

// The occurrence map: where and when occurrences happened, by type group --
// the first view of the intelligence layer. Codes are translated here; the API
// sends them as stored.
export default function OccurrenceMap() {
  const { t, i18n } = useTranslation();
  const [searchParams, setSearchParams] = useSearchParams();
  const year = Number(searchParams.get("year")) || undefined;
  // ?occurrence= selects one once the data is in (e.g. after registering it).
  const focusId = Number(searchParams.get("occurrence")) || undefined;
  const hidden = useMemo(
    () => new Set((searchParams.get("hide") ?? "").split(",").filter(Boolean)),
    [searchParams],
  );
  const mode: MapMode = searchParams.get("mode") === "heatmap" ? "heatmap" : "points";

  const [years, setYears] = useState<number[]>([]);
  const [data, setData] = useState<OccurrenceMapData | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [selected, setSelected] = useState<OccurrenceFeature | null>(null);

  function updateParams(patch: Record<string, string | undefined>) {
    setSearchParams((previous) => {
      const next = new URLSearchParams(previous);
      for (const [key, value] of Object.entries(patch)) {
        if (!value) next.delete(key);
        else next.set(key, value);
      }
      return next;
    });
  }

  useEffect(() => {
    listOccurrenceYears().then(setYears).catch(() => setYears([]));
  }, []);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError("");
    setSelected(null);
    getOccurrenceMap({ year })
      .then((result) => {
        if (cancelled) return;
        setData(result);
        if (focusId) setSelected(result.features.find((f) => f.properties.id === focusId) ?? null);
      })
      .catch((err) => {
        if (!cancelled)
          setError(err instanceof Error ? err.message : t("occurrenceMap.loadError"));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
    // focusId is read once per load, when the page opens on an occurrence.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [year, t]);

  // Groups are filtered here, not by the API, so toggling one is instant.
  const counts = useMemo(() => {
    const c: Record<string, number> = {};
    for (const f of data?.features ?? []) c[f.properties.group] = (c[f.properties.group] ?? 0) + 1;
    return c;
  }, [data]);
  const visible = useMemo<OccurrenceMapData | null>(
    () =>
      data && {
        ...data,
        features: data.features.filter((f) => !hidden.has(f.properties.group)),
      },
    [data, hidden],
  );

  // All / None: clearing every group first makes it quick to show just one or two.
  function setAllGroups(show: boolean) {
    updateParams({ hide: show ? undefined : GROUPS.join(",") });
    if (!show) setSelected(null);
  }

  function toggleGroup(group: string) {
    const next = new Set(hidden);
    if (next.has(group)) next.delete(group);
    else next.add(group);
    updateParams({ hide: [...next].join(",") || undefined });
    if (selected && next.has(selected.properties.group)) setSelected(null);
  }

  return (
    <>
      <PageMeta
        title={`${t("occurrenceMap.title")} | TrackID`}
        description={t("occurrenceMap.metaDescription")}
      />
      <PageBreadcrumb pageTitle={t("occurrenceMap.title")} />

      <ComponentCard title={t("occurrenceMap.title")} desc={t("occurrenceMap.subtitle")}>
        <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
          <div className="flex flex-wrap items-center gap-3">
            <label className="text-sm text-gray-500 dark:text-gray-400" htmlFor="occurrence-year">
              {t("occurrenceMap.year")}
            </label>
            <select
              id="occurrence-year"
              value={year ?? ""}
              onChange={(e) => updateParams({ year: e.target.value || undefined })}
              className="h-11 rounded-lg border border-gray-200 bg-white px-3 text-sm text-gray-700 shadow-theme-xs dark:border-gray-800 dark:bg-gray-900 dark:text-gray-300"
            >
              <option value="">{t("occurrenceMap.allYears")}</option>
              {years.map((y) => (
                <option key={y} value={y}>
                  {y}
                </option>
              ))}
            </select>

            <div className="inline-flex overflow-hidden rounded-lg border border-gray-200 dark:border-gray-800" role="group">
              {(["points", "heatmap"] as MapMode[]).map((m) => (
                <button
                  key={m}
                  type="button"
                  aria-pressed={mode === m}
                  onClick={() => updateParams({ mode: m === "points" ? undefined : m })}
                  className={`h-11 px-4 text-sm font-medium transition-colors ${
                    mode === m
                      ? "bg-brand-500 text-white"
                      : "bg-white text-gray-700 hover:bg-gray-50 dark:bg-gray-900 dark:text-gray-400 dark:hover:bg-white/[0.03]"
                  }`}
                >
                  {t(`occurrenceMap.${m}`)}
                </button>
              ))}
            </div>
          </div>

          <div className="flex items-center gap-4">
            <span className="text-sm text-gray-500 dark:text-gray-400">
              {loading
                ? t("occurrenceMap.loading")
                : t("occurrenceMap.count", { count: visible?.features.length ?? 0 })}
            </span>
            <Link
              to="/intelligence/occurrences/new"
              className="inline-flex h-11 items-center rounded-lg bg-brand-500 px-4 text-sm font-medium text-white hover:bg-brand-600"
            >
              {t("occurrenceMap.newOccurrence")}
            </Link>
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-2" aria-label={t("occurrenceMap.legend")}>
          {(
            [
              ["showAll", true, hidden.size === 0],
              ["showNone", false, hidden.size === GROUPS.length],
            ] as const
          ).map(([key, show, active]) => (
            <button
              key={key}
              type="button"
              onClick={() => setAllGroups(show)}
              disabled={active}
              className="rounded-full border border-gray-200 bg-white px-3 py-1.5 text-sm font-medium text-brand-500 transition-colors hover:bg-gray-50 disabled:cursor-default disabled:text-gray-400 disabled:hover:bg-white dark:border-gray-800 dark:bg-gray-900 dark:text-brand-400 dark:hover:bg-white/[0.03] dark:disabled:text-gray-600 dark:disabled:hover:bg-gray-900"
            >
              {t(`occurrenceMap.${key}`)}
            </button>
          ))}
          <span className="mx-1 h-5 w-px bg-gray-200 dark:bg-gray-800" aria-hidden />
          {GROUPS.map((g) => {
            const off = hidden.has(g);
            return (
              <button
                key={g}
                type="button"
                aria-pressed={!off}
                onClick={() => toggleGroup(g)}
                className={`inline-flex items-center gap-2 rounded-full border px-3 py-1.5 text-sm transition-opacity ${
                  off ? "opacity-40" : ""
                } border-gray-200 bg-white text-gray-700 dark:border-gray-800 dark:bg-gray-900 dark:text-gray-300`}
              >
                <span className="h-3 w-3 rounded-full" style={{ backgroundColor: GROUP_COLORS[g] }} aria-hidden />
                {t(`occurrenceGroup.${g}`)}
                <span className="text-gray-400">{counts[g] ?? 0}</span>
              </button>
            );
          })}
        </div>

        {error && (
          <p className="text-sm text-error-500" role="alert">
            {error}
          </p>
        )}
        {data?.truncated && (
          <p className="text-sm text-warning-600 dark:text-warning-400">{t("occurrenceMap.truncated")}</p>
        )}

        <div className="grid grid-cols-1 gap-4 xl:grid-cols-3">
          <div className="xl:col-span-2">
            <OccurrenceMapView
              data={visible}
              mode={mode}
              selectedId={selected?.properties.id}
              onSelect={setSelected}
            />
          </div>
          <OccurrenceDetails feature={selected} locale={i18n.language} />
        </div>
      </ComponentCard>
    </>
  );
}
