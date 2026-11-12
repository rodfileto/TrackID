import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import ComponentCard from "../common/ComponentCard";
import OccurrenceDetails from "./OccurrenceDetails";
import OccurrenceMapView, { type MapMode } from "./OccurrenceMapView";
import type { OccurrenceFeature, PresenceMap } from "../../services/occurrences";

interface Props {
  /** Loads where the person or cluster appears; run again when it changes. */
  load: () => Promise<PresenceMap>;
  /** What the map is of, for the card's wording: "person" or "cluster". */
  subject: "person" | "cluster";
}

// Where a person or a cluster appears: the occurrences of the cases it has traces in, as points or as a heatmap. It
// says how much it could not draw (cases with no occurrence, occurrences with no place), so the map is not taken for
// the whole of the presence. The point of a case is where it happened, not where the person lives or was seen.
export default function PresenceMapCard({ load, subject }: Props) {
  const { t, i18n } = useTranslation();
  const [data, setData] = useState<PresenceMap | null>(null);
  const [error, setError] = useState("");
  const [mode, setMode] = useState<MapMode>("points");
  const [selected, setSelected] = useState<OccurrenceFeature | null>(null);

  useEffect(() => {
    let stale = false;
    setData(null);
    setSelected(null);
    setError("");
    load()
      .then((d) => !stale && setData(d))
      .catch((err) => !stale && setError(err instanceof Error ? err.message : t("presence.loadError")));
    return () => {
      stale = true;
    };
    // load is rebuilt on every render of the page; the subject it loads is what changes.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [subject]);

  const drawn = data?.features.length ?? 0;
  const modes: MapMode[] = ["points", "heatmap"];
  return (
    <ComponentCard title={t("presence.title")} desc={t(`presence.desc.${subject}`)}>
      {error && (
        <p className="text-sm text-error-500" role="alert">
          {error}
        </p>
      )}
      {data && data.cases === 0 && <p className="text-sm text-gray-500 dark:text-gray-400">{t("presence.noCases")}</p>}
      {data && data.cases > 0 && (
        <div className="flex flex-col gap-3">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-sm text-gray-700 dark:text-gray-300">
              {t("presence.summary", { cases: data.cases, occurrences: data.occurrences, drawn })}
              {data.cases > data.withOccurrence && (
                <span className="block text-theme-xs text-gray-500 dark:text-gray-400">
                  {t("presence.noOccurrence", { count: data.cases - data.withOccurrence })}
                </span>
              )}
              {data.unplaced > 0 && (
                <span className="block text-theme-xs text-gray-500 dark:text-gray-400">
                  {t("presence.unplaced", { count: data.unplaced })}
                </span>
              )}
              {data.truncated && <span className="block text-theme-xs text-warning-600 dark:text-warning-400">{t("occurrenceMap.truncated")}</span>}
            </p>
            {drawn > 0 && (
              <div className="inline-flex rounded-lg border border-gray-200 p-0.5 dark:border-gray-800" role="group" aria-label={t("presence.mode")}>
                {modes.map((m) => (
                  <button
                    key={m}
                    type="button"
                    aria-pressed={mode === m}
                    onClick={() => setMode(m)}
                    className={`rounded-md px-3 py-1 text-sm ${
                      mode === m ? "bg-brand-500 text-white" : "text-gray-600 hover:bg-gray-50 dark:text-gray-400 dark:hover:bg-white/5"
                    }`}
                  >
                    {t(`presence.modes.${m}`)}
                  </button>
                ))}
              </div>
            )}
          </div>
          {drawn === 0 ? (
            <p className="text-sm text-gray-500 dark:text-gray-400">{t("presence.nothingToDraw")}</p>
          ) : (
            <div className="grid grid-cols-1 gap-4 xl:grid-cols-3">
              <div className="xl:col-span-2">
                <OccurrenceMapView
                  data={data}
                  mode={mode}
                  selectedId={selected?.properties.id}
                  onSelect={setSelected}
                  heightClass="h-[420px]"
                />
              </div>
              <OccurrenceDetails feature={selected} locale={i18n.language} />
            </div>
          )}
        </div>
      )}
    </ComponentCard>
  );
}
