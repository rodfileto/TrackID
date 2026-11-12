import { Link } from "react-router";
import { useTranslation } from "react-i18next";
import Badge from "../ui/badge/Badge";
import { GROUP_COLORS } from "./groupColors";
import { geocodeFlagLabels, geocodeSourceLabel } from "./geocodeLabels";
import type { OccurrenceFeature } from "../../services/occurrences";

function formatDateTime(iso: string, locale: string): string {
  return new Date(iso).toLocaleString(locale, {
    dateStyle: "medium",
    timeStyle: "short",
  });
}

// What an occurrence on a map says of itself: its type, report numbers, when and where, and its cases. `inSet` marks the
// cases a person's or a cluster's map is about.
export default function OccurrenceDetails({ feature, locale }: { feature: OccurrenceFeature | null; locale: string }) {
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
        {row(
          t("occurrenceMap.report"),
          [p.trackidNumber, p.agency && p.number ? `${p.agency} · ${p.number}` : ""].filter(Boolean).join(" — "),
        )}
        {p.occurredFrom && row(
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
        {geocodeSourceLabel(t, p.geocodeSource) && row(t("occurrenceMap.geocodeSource"), geocodeSourceLabel(t, p.geocodeSource))}
        {(p.geocodeFlags ?? []).length > 0 && row(t("occurrenceMap.geocodeCheck"), geocodeFlagLabels(t, p.geocodeFlags).join(", "))}
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
                {(p.inSet ?? []).includes(c.caseId) && <span className="h-2 w-2 rounded-full bg-brand-500" title={t("presence.inSet")} aria-label={t("presence.inSet")} />}
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
