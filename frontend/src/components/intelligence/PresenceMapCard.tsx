import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import Badge from "../ui/badge/Badge";
import ComponentCard from "../common/ComponentCard";
import OccurrenceDetails from "./OccurrenceDetails";
import OccurrenceMapView, { type MapMode } from "./OccurrenceMapView";
import type { OccurrenceFeature, PresenceArea, PresenceAreas, PresenceLinks, PresenceMap } from "../../services/occurrences";

interface Props {
  /** What the map is of, for the card's wording. */
  subject: "person" | "cluster";
  /** Changes when the person or cluster does, to load again. */
  subjectKey: string | number;
  loadMap: (links: PresenceLinks) => Promise<PresenceMap>;
  loadAreas: (links: PresenceLinks, epsM: number) => Promise<PresenceAreas>;
}

/** The distances areas are looked for at (docs/presence-areas-plan.md): a neighbourhood, a city, a region. */
const PRESETS = [
  { key: "district", epsM: 500 },
  { key: "city", epsM: 3000 },
  { key: "region", epsM: 25000 },
] as const;

const day = (iso?: string, locale?: string) => (iso ? new Date(iso).toLocaleDateString(locale, { dateStyle: "medium" }) : "");

const distance = (m: number) => (m >= 1000 ? `${(m / 1000).toFixed(m >= 10000 ? 0 : 1)} km` : `${Math.round(m)} m`);

// Where a person or a cluster appears: the occurrences of the cases it has traces in, facial and fingerprint together,
// as points or a heatmap, and optionally the areas where they gather. It says how much it could not draw (cases with no
// occurrence, occurrences with no place) so the map is not taken for the whole of the presence, and what an area means:
// where occurrences happened, not where anyone lives.
export default function PresenceMapCard({ subject, subjectKey, loadMap, loadAreas }: Props) {
  const { t, i18n } = useTranslation();
  const [links, setLinks] = useState<PresenceLinks>("all");
  const [epsM, setEpsM] = useState<number | null>(null);
  const [data, setData] = useState<PresenceMap | null>(null);
  const [areas, setAreas] = useState<PresenceAreas | null>(null);
  const [error, setError] = useState("");
  const [mode, setMode] = useState<MapMode>("points");
  const [selected, setSelected] = useState<OccurrenceFeature | null>(null);
  const [selectedArea, setSelectedArea] = useState<number | undefined>();
  const load = useRef({ loadMap, loadAreas });
  load.current = { loadMap, loadAreas };

  useEffect(() => {
    let stale = false;
    setData(null);
    setSelected(null);
    setError("");
    load.current
      .loadMap(links)
      .then((d) => !stale && setData(d))
      .catch((err) => !stale && setError(err instanceof Error ? err.message : t("presence.loadError")));
    return () => {
      stale = true;
    };
  }, [subjectKey, links, t]);

  useEffect(() => {
    let stale = false;
    setAreas(null);
    setSelectedArea(undefined);
    if (epsM === null) return;
    load.current
      .loadAreas(links, epsM)
      .then((a) => !stale && setAreas(a))
      .catch((err) => !stale && setError(err instanceof Error ? err.message : t("presence.loadError")));
    return () => {
      stale = true;
    };
  }, [subjectKey, links, epsM, t]);

  const drawn = data?.features.length ?? 0;
  const modes: MapMode[] = ["points", "heatmap"];
  const segmented = "inline-flex rounded-lg border border-gray-200 p-0.5 dark:border-gray-800";
  const segment = (on: boolean) =>
    `rounded-md px-3 py-1 text-sm ${on ? "bg-brand-500 text-white" : "text-gray-600 hover:bg-gray-50 dark:text-gray-400 dark:hover:bg-white/5"}`;

  return (
    <ComponentCard title={t("presence.title")} desc={t(`presence.desc.${subject}`)}>
      {error && (
        <p className="text-sm text-error-500" role="alert">
          {error}
        </p>
      )}

      <div className="flex flex-wrap items-center gap-x-6 gap-y-3">
        <div className={segmented} role="group" aria-label={t("presence.linksLabel")}>
          {(["all", "reviewed"] as const).map((l) => (
            <button key={l} type="button" aria-pressed={links === l} onClick={() => setLinks(l)} className={segment(links === l)}>
              {t(`presence.links.${l}`)}
            </button>
          ))}
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-theme-xs text-gray-500 dark:text-gray-400">{t("presence.areasLabel")}</span>
          <div className={segmented} role="group" aria-label={t("presence.areasLabel")}>
            <button type="button" aria-pressed={epsM === null} onClick={() => setEpsM(null)} className={segment(epsM === null)}>
              {t("presence.noAreas")}
            </button>
            {PRESETS.map((p) => (
              <button key={p.key} type="button" aria-pressed={epsM === p.epsM} onClick={() => setEpsM(p.epsM)} className={segment(epsM === p.epsM)}>
                {t(`presence.presets.${p.key}`)} · {distance(p.epsM)}
              </button>
            ))}
          </div>
        </div>
        {drawn > 0 && (
          <div className={segmented} role="group" aria-label={t("presence.mode")}>
            {modes.map((m) => (
              <button key={m} type="button" aria-pressed={mode === m} onClick={() => setMode(m)} className={segment(mode === m)}>
                {t(`presence.modes.${m}`)}
              </button>
            ))}
          </div>
        )}
      </div>

      {data && data.cases === 0 && (
        <p className="text-sm text-gray-500 dark:text-gray-400">
          {links === "reviewed" && data.basis === "AUTOMATIC" ? t("presence.automaticCluster") : t(links === "reviewed" ? "presence.noReviewed" : "presence.noCases")}
        </p>
      )}

      {data && data.cases > 0 && (
        <div className="flex flex-col gap-3">
          <p className="text-sm text-gray-700 dark:text-gray-300">
            {t("presence.summary", { cases: data.cases, occurrences: data.occurrences, drawn })}
            <span className="block text-theme-xs text-gray-500 dark:text-gray-400">
              {t("presence.modalities", { facial: data.facial, fingerprint: data.fingerprint })}
              {data.fused > 0 && <> · <span className="text-purple-600 dark:text-purple-400">{t("presence.fused", { count: data.fused })}</span></>}
            </span>
            {data.cases > data.withOccurrence && (
              <span className="block text-theme-xs text-gray-500 dark:text-gray-400">{t("presence.noOccurrence", { count: data.cases - data.withOccurrence })}</span>
            )}
            {data.unplaced > 0 && (
              <span className="block text-theme-xs text-gray-500 dark:text-gray-400">{t("presence.unplaced", { count: data.unplaced })}</span>
            )}
            {data.truncated && <span className="block text-theme-xs text-warning-600 dark:text-warning-400">{t("occurrenceMap.truncated")}</span>}
          </p>

          {drawn === 0 ? (
            <p className="text-sm text-gray-500 dark:text-gray-400">{t("presence.nothingToDraw")}</p>
          ) : (
            <>
              <div className="grid grid-cols-1 gap-4 xl:grid-cols-3">
                <div className="xl:col-span-2">
                  <OccurrenceMapView
                    data={data}
                    mode={mode}
                    selectedId={selected?.properties.id}
                    onSelect={setSelected}
                    heightClass="h-[420px]"
                    areas={areas?.areas ?? null}
                    selectedArea={selectedArea}
                  />
                  <p className="mt-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-theme-xs text-gray-500 dark:text-gray-400">
                    <span className="inline-flex items-center gap-1.5">
                      <span className="inline-block h-3 w-3 rounded-full border-2 border-purple-600" aria-hidden /> {t("presence.legend.fused")}
                    </span>
                    <span className="inline-flex items-center gap-1.5">
                      <span className="inline-block h-3 w-3 rounded-full bg-brand-500/50" aria-hidden /> {t("presence.legend.unconfirmed")}
                    </span>
                    {areas && (
                      <span className="inline-flex items-center gap-1.5">
                        <span className="inline-block h-3 w-3 border border-dashed border-amber-600 bg-amber-500/20" aria-hidden /> {t("presence.legend.area")}
                      </span>
                    )}
                  </p>
                </div>
                <OccurrenceDetails feature={selected} locale={i18n.language} />
              </div>

              {epsM !== null && areas && <Areas areas={areas} selected={selectedArea} onSelect={setSelectedArea} locale={i18n.language} />}
              {areas && areas.municipalities.length > 0 && <Municipalities areas={areas} />}
            </>
          )}
        </div>
      )}
    </ComponentCard>
  );
}

function Areas({ areas, selected, onSelect, locale }: { areas: PresenceAreas; selected?: number; onSelect: (rank?: number) => void; locale: string }) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-2 rounded-xl border border-gray-200 p-4 dark:border-gray-800">
      <h4 className="text-sm font-medium text-gray-800 dark:text-white/90">{t("presence.areasTitle", { eps: distance(areas.epsM) })}</h4>
      {areas.tooFew && <p className="text-theme-xs text-warning-600 dark:text-warning-400">{t("presence.tooFew", { count: areas.mapped })}</p>}
      {areas.areas.length === 0 ? (
        <p className="text-sm text-gray-500 dark:text-gray-400">{t("presence.noAreasFound", { eps: distance(areas.epsM) })}</p>
      ) : (
        <ul className="flex flex-col divide-y divide-gray-100 dark:divide-gray-800">
          {areas.areas.map((a) => (
            <AreaRow key={a.rank} area={a} mapped={areas.mapped} selected={selected === a.rank} onSelect={() => onSelect(selected === a.rank ? undefined : a.rank)} locale={locale} />
          ))}
        </ul>
      )}
      {areas.noise.length > 0 && <p className="text-theme-xs text-gray-500 dark:text-gray-400">{t("presence.isolated", { count: areas.noise.length })}</p>}
      <p className="text-theme-xs text-gray-500 dark:text-gray-400">{t("presence.areasNote")}</p>
    </div>
  );
}

function AreaRow({ area, mapped, selected, onSelect, locale }: { area: PresenceArea; mapped: number; selected: boolean; onSelect: () => void; locale: string }) {
  const { t } = useTranslation();
  const n = area.pointIds.length;
  const period = area.first && area.last ? (day(area.first, locale) === day(area.last, locale) ? day(area.first, locale) : `${day(area.first, locale)} – ${day(area.last, locale)}`) : "";
  return (
    <li>
      <button
        type="button"
        onClick={onSelect}
        aria-pressed={selected}
        className={`flex w-full flex-col gap-1 px-2 py-2 text-left hover:bg-gray-50 dark:hover:bg-white/5 ${selected ? "bg-brand-50 dark:bg-brand-500/10" : ""}`}
      >
        <span className="flex flex-wrap items-center gap-2 text-sm text-gray-800 dark:text-white/90">
          <span className="font-medium">{t("presence.area", { rank: area.rank })}</span>
          {area.main && <Badge size="sm" color="warning">{t("presence.main")}</Badge>}
          <span>{t("presence.areaCount", { count: n, share: Math.round(area.share * 100), total: mapped })}</span>
          <span className="text-gray-500 dark:text-gray-400">{t("presence.radius", { radius: distance(area.radiusM) })}</span>
        </span>
        <span className="text-theme-xs text-gray-500 dark:text-gray-400">
          {[
            period,
            area.types.map((x) => `${t(`occurrenceType.${x.type}`)} ×${x.count}`).join(", "),
            t("presence.reviewedOf", { reviewed: area.reviewed, count: n }),
            area.fused > 0 ? t("presence.fused", { count: area.fused }) : "",
          ]
            .filter(Boolean)
            .join(" · ")}
        </span>
        <span className="text-theme-xs text-gray-500 dark:text-gray-400">
          {area.quotient !== undefined
            ? t("presence.quotient", { value: area.quotient.toFixed(1), base: area.baseInside })
            : t("presence.noQuotient", { base: area.baseInside })}
        </span>
      </button>
    </li>
  );
}

function Municipalities({ areas }: { areas: PresenceAreas }) {
  const { t } = useTranslation();
  const max = Math.max(...areas.municipalities.map((m) => m.count));
  return (
    <div className="flex flex-col gap-2 rounded-xl border border-gray-200 p-4 dark:border-gray-800">
      <h4 className="text-sm font-medium text-gray-800 dark:text-white/90">{t("presence.byMunicipality")}</h4>
      <ul className="flex flex-col gap-1.5">
        {areas.municipalities.slice(0, 10).map((m) => (
          <li key={m.code} className="flex items-center gap-3 text-sm">
            <span className="w-48 shrink-0 truncate text-gray-700 dark:text-gray-300" title={m.name}>
              {m.name}/{m.uf}
            </span>
            <span className="h-2 flex-1 rounded bg-gray-100 dark:bg-gray-800" aria-hidden>
              <span className="block h-2 rounded bg-brand-500" style={{ width: `${(m.count / max) * 100}%` }} />
            </span>
            <span className="w-8 text-right tabular-nums text-gray-600 dark:text-gray-400">{m.count}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}
