import { useTranslation } from "react-i18next";

import type { Conclusion } from "../../services/analyses";
import type { FaceLabInfo, Opinion, Summary, Warning } from "../../services/facelab";
import { StabilityChip } from "./controls";
import { inputClass, useLabel } from "./style";

// The evaluation step: the findings counted (a help, not the opinion), the warnings, and the
// opinion on the OSAC 2022-S-0001 scale FISWG refers to, with its reasoning.

export default function EvaluationPanel({ info, summary, warnings, opinion, onOpinion }: {
  info: FaceLabInfo;
  summary: Summary | null;
  warnings: Warning[];
  opinion?: Opinion;
  onOpinion: (o: Opinion | undefined) => void;
}) {
  const { t } = useTranslation();
  const label = useLabel();
  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <div className="flex flex-col gap-3">
        <span className="text-base font-medium text-gray-800 dark:text-white/90">{t("faceLab.evaluation.summary")}</span>
        {summary ? (
          <div className="flex flex-col gap-2 text-sm text-gray-700 dark:text-gray-300">
            <p>{t("faceLab.evaluation.counts", { similar: summary.similar, dissimilar: summary.dissimilar, explained: summary.explained, notComparable: summary.notComparable })}</p>
            <p>{t("faceLab.evaluation.components", { count: summary.componentsCompared, total: info.components.length })}</p>
            <table className="w-max text-sm">
              <thead>
                <tr className="text-gray-500 dark:text-gray-400">
                  <th className="pr-4 text-left font-normal">{t("faceLab.evaluation.stability")}</th>
                  {(["H", "M", "L", ""] as const).map((s) => <th key={s} className="px-2 font-normal"><StabilityChip value={s} /></th>)}
                </tr>
              </thead>
              <tbody>
                <tr><td className="pr-4">{t("faceLab.evaluation.similar")}</td>{(["H", "M", "L", ""] as const).map((s) => <td key={s} className="px-2 text-center">{summary.similarByStability[s] ?? 0}</td>)}</tr>
                <tr><td className="pr-4">{t("faceLab.evaluation.dissimilar")}</td>{(["H", "M", "L", ""] as const).map((s) => <td key={s} className="px-2 text-center">{summary.dissimilarByStability[s] ?? 0}</td>)}</tr>
              </tbody>
            </table>
            <p className="text-gray-500 dark:text-gray-400">{t("faceLab.evaluation.notOpinion")}</p>
          </div>
        ) : (
          <p className="text-sm text-gray-500 dark:text-gray-400">…</p>
        )}

        <span className="mt-2 text-base font-medium text-gray-800 dark:text-white/90">{t("faceLab.evaluation.warnings")}</span>
        {warnings.length === 0 ? (
          <p className="text-sm text-success-600">{t("faceLab.evaluation.noWarnings")}</p>
        ) : (
          <ul className="flex flex-col gap-1 text-sm text-warning-700 dark:text-warning-400">
            {warnings.map((w, i) => <li key={i}>• {w.message}</li>)}
          </ul>
        )}
      </div>

      <div className="flex flex-col gap-3">
        <span className="text-base font-medium text-gray-800 dark:text-white/90">{t("faceLab.evaluation.opinion")}</span>
        <p className="text-sm text-gray-500 dark:text-gray-400">{t("faceLab.evaluation.scaleHint")}</p>
        {info.scale.map((l) => (
          <label key={l.code} className="flex items-start gap-2 text-sm text-gray-700 dark:text-gray-300">
            <input type="radio" className="mt-0.5 accent-brand-500" checked={opinion?.conclusion === l.code}
              onChange={() => onOpinion({ conclusion: l.code as Conclusion, reasoning: opinion?.reasoning ?? "" })} />
            <span>
              <span className="font-medium text-gray-800 dark:text-white/90">{label(l)}</span>
              <span className="block text-gray-500 dark:text-gray-400">{l.meaningPt}</span>
            </span>
          </label>
        ))}
        {opinion && (
          <>
            <textarea rows={4} value={opinion.reasoning} placeholder={t("faceLab.evaluation.reasoning")}
              onChange={(e) => onOpinion({ ...opinion, reasoning: e.target.value })} className={`${inputClass} text-sm`} />
            <button type="button" className="self-start text-sm text-gray-500 underline dark:text-gray-400" onClick={() => onOpinion(undefined)}>
              {t("faceLab.evaluation.clear")}
            </button>
          </>
        )}
        <p className="text-sm text-gray-500 dark:text-gray-400">{t("faceLab.evaluation.noVerification")}</p>
      </div>
    </div>
  );
}
