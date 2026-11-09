import { useState } from "react";
import { useTranslation } from "react-i18next";

import type { Characteristic, FaceLabInfo, Finding, FindingResult, StabilityFactor } from "../../services/facelab";
import { StabilityChip } from "./controls";
import { buttonClass, inputClass, useLabel } from "./style";

// The comparison step: FISWG's feature list (ASTM E3149), top of the face to the bottom, each
// characteristic with its stability (FISWG Physical Stability of Facial Features of Adults) and
// the findings recorded on it.

const UNEXPLAINED = "UNEXPLAINED";

const MULTIPLE = new Set(["SCAR", "SKIN_MARK", "OTHER_CREASES", "OTHER_ALTERATION", "OTHER"]);

function weakest(ch: Characteristic, factors: StabilityFactor[]) {
  if (!ch.stability) return undefined;
  const rank = { H: 3, M: 2, L: 1 } as const;
  let worst: "H" | "M" | "L" | undefined;
  for (const f of factors) {
    const s = ch.stability[f];
    if (s && (!worst || rank[s] < rank[worst])) worst = s;
  }
  return worst;
}

function FindingEditor({ info, ch, finding, numbers, onChange, onRemove }: {
  info: FaceLabInfo; ch: Characteristic; finding: Finding; numbers: number[];
  onChange: (f: Finding) => void; onRemove: () => void;
}) {
  const { t } = useTranslation();
  const label = useLabel();
  const set = (patch: Partial<Finding>) => onChange({ ...finding, ...patch });
  const toggle = <T,>(list: T[] | undefined, v: T) => (list ?? []).includes(v) ? (list ?? []).filter((x) => x !== v) : [...(list ?? []), v];
  return (
    <div className="flex flex-col gap-2 rounded-lg border border-gray-200 p-3 dark:border-gray-800">
      <div className="flex flex-wrap items-center gap-3">
        {ch.sided && (
          <span className="text-sm font-medium text-gray-700 dark:text-gray-300">{t(`faceLab.findings.side.${finding.side}`)}</span>
        )}
        {info.results.map((r) => (
          <label key={r.code} className="flex items-center gap-1 text-sm text-gray-700 dark:text-gray-300">
            <input type="radio" checked={finding.result === r.code} className="accent-brand-500"
              onChange={() => set({ result: r.code as FindingResult, explanations: r.code === "DISSIMILAR" ? finding.explanations : undefined })} />
            {label(r)}
          </label>
        ))}
        <button type="button" onClick={onRemove} className="ml-auto text-sm text-error-500 hover:underline">{t("faceLab.findings.remove")}</button>
      </div>
      <div className="flex flex-wrap gap-x-3 gap-y-1">
        {ch.descriptors.map((d) => (
          <label key={d} className="flex items-center gap-1 text-sm text-gray-600 dark:text-gray-400">
            <input type="checkbox" checked={(finding.descriptors ?? []).includes(d)} onChange={() => set({ descriptors: toggle(finding.descriptors, d) })} className="accent-brand-500" />
            {label(info.descriptors[d])}
          </label>
        ))}
      </div>
      <div className="grid gap-2 md:grid-cols-2">
        <textarea rows={2} value={finding.questioned ?? ""} placeholder={t("faceLab.findings.inQ")} onChange={(e) => set({ questioned: e.target.value })} className={`${inputClass} text-sm`} />
        <textarea rows={2} value={finding.known ?? ""} placeholder={t("faceLab.findings.inK")} onChange={(e) => set({ known: e.target.value })} className={`${inputClass} text-sm`} />
      </div>
      {finding.result === "DISSIMILAR" && (
        // One or more causes, or "not explained" alone.
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm text-gray-600 dark:text-gray-400">
          {t("faceLab.findings.explanation")}
          {info.explanations.map((e) => (
            <label key={e.code} className="flex items-center gap-1">
              <input type="checkbox" className="accent-brand-500" checked={(finding.explanations ?? []).includes(e.code)}
                onChange={() => {
                  const list = finding.explanations ?? [];
                  const next = e.code === UNEXPLAINED
                    ? (list.includes(UNEXPLAINED) ? [] : [UNEXPLAINED])
                    : toggle(list.filter((x) => x !== UNEXPLAINED), e.code);
                  set({ explanations: next.length ? next : undefined });
                }} />
              {label(e)}
            </label>
          ))}
        </div>
      )}
      <div className="flex flex-wrap items-center gap-2 text-sm text-gray-600 dark:text-gray-400">
        {t("faceLab.findings.markups")}
        {numbers.length === 0 && <span className="text-gray-400">{t("faceLab.findings.noMarkups")}</span>}
        {numbers.map((n) => (
          <label key={n} className="flex items-center gap-1">
            <input type="checkbox" checked={(finding.markups ?? []).includes(n)} onChange={() => set({ markups: toggle(finding.markups, n).sort((a, b) => a - b) })} className="accent-brand-500" />
            {n}
          </label>
        ))}
      </div>
      <input value={finding.notes ?? ""} placeholder={t("faceLab.findings.notes")} onChange={(e) => set({ notes: e.target.value })} className={`${inputClass} text-sm`} />
    </div>
  );
}

export default function FindingsPanel({ info, findings, factors, numbers, onFindings, onFactors }: {
  info: FaceLabInfo;
  findings: Finding[];
  factors: StabilityFactor[];
  numbers: number[];
  onFindings: (f: Finding[]) => void;
  onFactors: (f: StabilityFactor[]) => void;
}) {
  const { t } = useTranslation();
  const label = useLabel();
  const [open, setOpen] = useState<string | null>(info.components[0]?.code ?? null);

  function add(ch: Characteristic, side: "" | "RIGHT" | "LEFT") {
    onFindings([...findings, { characteristic: ch.code, side: side || undefined, result: "SIMILAR" }]);
  }

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-col gap-2 rounded-lg bg-gray-50 p-3 dark:bg-white/[0.03]">
        <span className="text-base font-medium text-gray-800 dark:text-white/90">{t("faceLab.findings.factorsTitle")}</span>
        <p className="text-sm text-gray-500 dark:text-gray-400">{t("faceLab.findings.factorsHint")}</p>
        <div className="flex flex-wrap gap-x-4 gap-y-1">
          {info.stabilityFactors.map((f) => (
            <label key={f.code} className="flex items-center gap-1.5 text-sm text-gray-700 dark:text-gray-300">
              <input type="checkbox" className="accent-brand-500" checked={factors.includes(f.code as StabilityFactor)}
                onChange={(e) => onFactors(e.target.checked ? [...factors, f.code as StabilityFactor] : factors.filter((x) => x !== f.code))} />
              {label(f)}
            </label>
          ))}
        </div>
        <p className="flex flex-wrap items-center gap-2 text-sm text-gray-500 dark:text-gray-400">
          {t("faceLab.findings.legend")}
          <StabilityChip value="H" label={t("faceLab.findings.high")} />
          <StabilityChip value="M" label={t("faceLab.findings.medium")} />
          <StabilityChip value="L" label={t("faceLab.findings.low")} />
        </p>
      </div>

      {info.components.map((comp) => {
        const count = findings.filter((f) => comp.characteristics.some((c) => c.code === f.characteristic)).length;
        const isOpen = open === comp.code;
        return (
          <div key={comp.code} className="rounded-lg border border-gray-200 dark:border-gray-800">
            <button type="button" onClick={() => setOpen(isOpen ? null : comp.code)}
              className="flex w-full items-center justify-between px-3 py-2 text-left text-base font-medium text-gray-800 dark:text-white/90">
              <span>{label(comp)}</span>
              <span className="text-sm font-normal text-gray-500 dark:text-gray-400">{count > 0 && t("faceLab.findings.count", { count })} {isOpen ? "▾" : "▸"}</span>
            </button>
            {isOpen && (
              <div className="flex flex-col gap-3 border-t border-gray-100 px-3 py-3 dark:border-gray-800">
                {comp.characteristics.map((ch) => {
                  const mine = findings.map((f, i) => ({ f, i })).filter(({ f }) => f.characteristic === ch.code);
                  const sides: ("" | "RIGHT" | "LEFT")[] = ch.sided ? ["RIGHT", "LEFT"] : [""];
                  const w = weakest(ch, factors);
                  return (
                    <div key={ch.code} className="flex flex-col gap-2">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="text-base text-gray-800 dark:text-white/90">{label(ch)}</span>
                        {ch.stability ? (
                          <span className="flex flex-wrap items-center gap-1" title={t("faceLab.findings.stabilityTitle", { page: ch.stability.page })}>
                            {info.stabilityFactors.map((f) => (
                              <StabilityChip key={f.code} value={ch.stability![f.code as StabilityFactor]} title={label(f)}
                                label={t(`faceLab.findings.short.${f.code}`)}
                                dim={factors.length > 0 && !factors.includes(f.code as StabilityFactor)} />
                            ))}
                          </span>
                        ) : (
                          <span className="text-xs text-gray-400">{t("faceLab.findings.noRating")}</span>
                        )}
                        {w && <span className="text-xs text-gray-500 dark:text-gray-400">{t("faceLab.findings.weakest")} <StabilityChip value={w} /></span>}
                        <span className="ml-auto flex gap-1">
                          {sides.map((side) => {
                            const taken = !MULTIPLE.has(ch.code) && mine.some(({ f }) => (f.side ?? "") === side);
                            return (
                              <button key={side} type="button" disabled={taken} onClick={() => add(ch, side)} className={buttonClass}>
                                {side ? t(`faceLab.findings.add${side}`) : t("faceLab.findings.add")}
                              </button>
                            );
                          })}
                        </span>
                      </div>
                      {mine.map(({ f, i }) => (
                        <FindingEditor key={i} info={info} ch={ch} finding={f} numbers={numbers}
                          onChange={(nf) => onFindings(findings.map((x, j) => (j === i ? nf : x)))}
                          onRemove={() => onFindings(findings.filter((_, j) => j !== i))} />
                      ))}
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        );
      })}
      <p className="text-sm text-gray-500 dark:text-gray-400">{info.featureListSource}. {info.stabilitySource}.</p>
    </div>
  );
}
