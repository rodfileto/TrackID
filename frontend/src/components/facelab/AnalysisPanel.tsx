import { useTranslation } from "react-i18next";

import type { Analysis, FaceLabInfo, FaceSource, ImageFactor, Metrics } from "../../services/facelab";
import { Section, Toggle } from "./controls";
import { inputClass, useLabel } from "./style";

// The ACE-V analysis of one image (FISWG ACE-V one-to-one guideline, §5.1): metadata, the image
// factors, alterations and suitability. The numbers describe the image, never the face.

export default function AnalysisPanel({ info, source, metrics, analysis, onChange }: {
  info: FaceLabInfo;
  source: FaceSource;
  metrics: Metrics | null;
  analysis: Analysis;
  onChange: (a: Analysis) => void;
}) {
  const { t } = useTranslation();
  const label = useLabel();
  const set = (patch: Partial<Analysis>) => onChange({ ...analysis, ...patch });
  const factor = (code: string): ImageFactor => analysis.factors.find((f) => f.code === code) ?? { code, effect: "NONE" };
  const setFactor = (f: ImageFactor) => set({ factors: [...analysis.factors.filter((x) => x.code !== f.code), f].sort(
    (a, b) => info.imageFactors.findIndex((l) => l.code === a.code) - info.imageFactors.findIndex((l) => l.code === b.code)) });
  const x = source.exif;
  const pose = metrics?.pose;

  return (
    <div className="flex flex-col gap-3 text-base">
      <Section title={t("faceLab.analysis.metadata")}>
        <dl className="grid grid-cols-[9rem_1fr] gap-x-3 gap-y-1 text-sm text-gray-600 dark:text-gray-400">
          <dt>{t("faceLab.analysis.file")}</dt><dd className="text-gray-800 dark:text-white/90">{source.filename} · {source.format.toUpperCase()} · {source.width} × {source.height} px</dd>
          <dt>SHA-256</dt><dd className="break-all font-mono">{source.sha256}</dd>
          <dt>{t("faceLab.analysis.captured")}</dt><dd>{x.dateTimeOriginal || "—"}</dd>
          <dt>{t("faceLab.analysis.camera")}</dt><dd>{[x.make, x.model].filter(Boolean).join(" ") || "—"}</dd>
          <dt>{t("faceLab.analysis.software")}</dt><dd className={x.software ? "text-warning-600" : ""}>{x.software || "—"}</dd>
          {x.dateTime && x.dateTime !== x.dateTimeOriginal && (<><dt>{t("faceLab.analysis.modified")}</dt><dd>{x.dateTime}</dd></>)}
        </dl>
        <p className="text-sm text-gray-500 dark:text-gray-400">{t("faceLab.analysis.exifNote")}</p>
      </Section>

      <Section title={t("faceLab.analysis.measures")}>
        {metrics ? (
          <dl className="grid grid-cols-[9rem_1fr] gap-x-3 gap-y-1 text-sm text-gray-600 dark:text-gray-400">
            {pose && (
              <>
                <dt>{t("faceLab.analysis.ipd")}</dt>
                <dd className={pose.ipd < info.lowIpd ? "font-medium text-warning-600" : "text-gray-800 dark:text-white/90"}>
                  {pose.ipd.toFixed(0)} px{pose.ipd < info.lowIpd && ` — ${t("faceLab.analysis.lowIpd")}`}
                </dd>
                <dt>{t("faceLab.analysis.roll")}</dt><dd>{pose.roll.toFixed(1)}°</dd>
                <dt>{t("faceLab.analysis.yaw")}</dt><dd>{pose.yaw.toFixed(0)}° <span className="text-gray-400">({t("faceLab.analysis.rough")})</span></dd>
              </>
            )}
            <dt>{t("faceLab.analysis.region")}</dt><dd>{metrics.width} × {metrics.height} px</dd>
            <dt>{t("faceLab.analysis.sharpness")}</dt><dd>{metrics.sharpness.toFixed(0)}</dd>
            <dt>{t("faceLab.analysis.clipped")}</dt><dd>{(100 * metrics.clippedDark).toFixed(1)}% / {(100 * metrics.clippedBright).toFixed(1)}%</dd>
          </dl>
        ) : (
          <p className="text-sm text-gray-500 dark:text-gray-400">{t("faceLab.analysis.noMeasures")}</p>
        )}
        <p className="text-sm text-gray-500 dark:text-gray-400">{t("faceLab.analysis.noAnthropometry")}</p>
      </Section>

      <Section title={t("faceLab.analysis.factors")}>
        {info.imageFactors.map((l) => {
          const f = factor(l.code);
          return (
            <div key={l.code} className="grid gap-1 md:grid-cols-[1fr_11rem]">
              <span className="text-sm text-gray-700 dark:text-gray-300">{label(l)}</span>
              <select value={f.effect} onChange={(e) => setFactor({ ...f, effect: e.target.value as ImageFactor["effect"] })} className={`${inputClass} text-sm`}>
                {info.effects.map((e) => <option key={e.code} value={e.code}>{label(e)}</option>)}
              </select>
              {f.effect !== "NONE" && (
                <input value={f.notes ?? ""} placeholder={t("faceLab.analysis.factorNote")} onChange={(e) => setFactor({ ...f, notes: e.target.value })}
                  className={`${inputClass} text-sm md:col-span-2`} />
              )}
            </div>
          );
        })}
      </Section>

      <Section title={t("faceLab.analysis.alterations")}>
        <Toggle label={t("faceLab.analysis.mirrored")} checked={analysis.mirrored} onChange={(v) => set({ mirrored: v })} />
        <Toggle label={t("faceLab.analysis.manipulation")} checked={analysis.manipulationSuspected} onChange={(v) => set({ manipulationSuspected: v })} />
        {analysis.manipulationSuspected && (
          <>
            <input value={analysis.manipulationNotes ?? ""} placeholder={t("faceLab.analysis.manipulationNote")}
              onChange={(e) => set({ manipulationNotes: e.target.value })} className={`${inputClass} text-sm`} />
            <p className="text-sm text-warning-600">{t("faceLab.analysis.manipulationRefer")}</p>
          </>
        )}
        <label className="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-400">
          {t("faceLab.analysis.captureDate")}
          <input value={analysis.captureDate ?? ""} onChange={(e) => set({ captureDate: e.target.value })} placeholder={x.dateTimeOriginal ?? ""} className={`${inputClass} text-sm`} />
        </label>
      </Section>

      <Section title={t("faceLab.analysis.suitability")}>
        <div className="flex flex-wrap gap-3">
          {info.suitabilities.map((s) => (
            <label key={s.code} className="flex items-center gap-1.5 text-sm text-gray-700 dark:text-gray-300">
              <input type="radio" checked={analysis.suitability === s.code} onChange={() => set({ suitability: s.code as Analysis["suitability"] })} className="accent-brand-500" />
              {label(s)}
            </label>
          ))}
        </div>
        <input value={analysis.suitabilityReason ?? ""} placeholder={t("faceLab.analysis.suitabilityReason")}
          onChange={(e) => set({ suitabilityReason: e.target.value })} className={`${inputClass} text-sm`} />
        <textarea value={analysis.notes ?? ""} placeholder={t("faceLab.analysis.notes")} rows={2}
          onChange={(e) => set({ notes: e.target.value })} className={`${inputClass} text-sm`} />
        <Toggle label={<span className="font-medium">{t("faceLab.analysis.done")}</span>} checked={analysis.done}
          disabled={!analysis.suitability} onChange={(v) => set({ done: v })} />
      </Section>
    </div>
  );
}
