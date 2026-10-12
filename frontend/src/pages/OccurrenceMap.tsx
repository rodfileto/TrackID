import { useEffect, useMemo, useState } from "react";
import { Link, useSearchParams } from "react-router";
import { useTranslation } from "react-i18next";
import PageMeta from "../components/common/PageMeta";
import PageBreadcrumb from "../components/common/PageBreadCrumb";
import ComponentCard from "../components/common/ComponentCard";
import Badge from "../components/ui/badge/Badge";
import OccurrenceMapView, {
  type MapMode,
} from "../components/intelligence/OccurrenceMapView";
import { GROUP_COLORS } from "../components/intelligence/groupColors";
import {
  getOccurrenceMap,
  listOccurrenceYears,
  type OccurrenceFeature,
  type OccurrenceMap as OccurrenceMapData,
} from "../services/occurrences";

const GROUPS = Object.keys(GROUP_COLORS);

function formatDateTime(iso: string, locale: string): string {
  return new Date(iso).toLocaleString(locale, {
    dateStyle: "medium",
    timeStyle: "short",
  });
}

// The occurrence map: where and when occurrences happened, by type group --
// the first view of the intelligence layer. Codes are translated here; the API
// sends them as stored.
export default function OccurrenceMap() {
  const { t, i18n } = useTranslation();
  const [searchParams, setSearchParams] = useSearchParams();
  const year = Number(searchParams.get("year")) || undefined;
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
        if (!cancelled) setData(result);
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

          <span className="text-sm text-gray-500 dark:text-gray-400">
            {loading
              ? t("occurrenceMap.loading")
              : t("occurrenceMap.count", { count: visible?.features.length ?? 0 })}
          </span>
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

function OccurrenceDetails({ feature, locale }: { feature: OccurrenceFeature | null; locale: string }) {
  const { t } = useTranslation();
  if (!feature) {
    return (
      <div className="flex min-h-40 items-center justify-center rounded-xl border border-dashed border-gray-200 p-6 text-center text-sm text-gray-500 dark:border-gray-800 dark:text-gray-400">
        {t("occurrenceMap.selectHint")}
      </div>
    );
  }
  const p = feature.properties;
  const row = (label: string, value: React.ReactNode) => (
    <div className="flex flex-col gap-0.5">
      <dt className="text-theme-xs text-gray-500 dark:text-gray-400">{label}</dt>
      <dd className="text-sm text-gray-800 dark:text-white/90">{value}</dd>
    </div>
  );
  return (
    <div className="flex flex-col gap-4 rounded-xl border border-gray-200 p-5 dark:border-gray-800">
      <div className="flex flex-wrap items-center gap-2">
        <span className="h-3 w-3 rounded-full" style={{ backgroundColor: GROUP_COLORS[p.group] ?? "#6b7280" }} aria-hidden />
        <h4 className="text-base font-medium text-gray-800 dark:text-white/90">
          {p.types.map((c) => t(`occurrenceType.${c}`)).join(" · ")}
        </h4>
        {p.attempted && <Badge color="warning" size="sm">{t("occurrenceMap.attempted")}</Badge>}
      </div>
      <dl className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-1">
        {row(t("occurrenceMap.report"), `${p.agency} · ${p.number}`)}
        {row(
          t("occurrenceMap.when"),
          p.occurredTo
            ? `${formatDateTime(p.occurredFrom, locale)} – ${formatDateTime(p.occurredTo, locale)}`
            : formatDateTime(p.occurredFrom, locale),
        )}
        {row(t("occurrenceMap.status"), t(`occurrenceStatus.${p.status}`))}
        {row(
          t("occurrenceMap.place"),
          [p.placeName, p.neighbourhood, p.placeType && t(`placeType.${p.placeType}`)].filter(Boolean).join(" · ") ||
            "—",
        )}
        {row(t("occurrenceMap.precision"), t(`precision.${p.precision}`))}
        {p.modusOperandi.length > 0 &&
          row(t("occurrenceMap.modusOperandi"), p.modusOperandi.map((m) => t(`modus.${m}`, { defaultValue: m })).join(", "))}
      </dl>
      <div className="flex flex-col gap-2">
        <h5 className="text-theme-xs text-gray-500 dark:text-gray-400">
          {t("occurrenceMap.cases", { count: p.cases.length })}
        </h5>
        {p.cases.length === 0 ? (
          <span className="text-sm text-gray-400 dark:text-white/30">{t("occurrenceMap.noCases")}</span>
        ) : (
          <ul className="flex flex-col gap-1.5">
            {p.cases.map((c) => (
              <li key={c.caseId} className="flex items-center gap-2">
                <Link
                  to={`/cases/${encodeURIComponent(c.caseId)}`}
                  className="font-medium text-brand-500 hover:text-brand-600 dark:text-brand-400 dark:hover:text-brand-300"
                >
                  {c.caseId}
                </Link>
                <Badge color={c.modality === "FACIAL" ? "info" : "warning"} size="sm">
                  {t(c.modality === "FACIAL" ? "modality.facial" : "modality.fingerprint")}
                </Badge>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}
