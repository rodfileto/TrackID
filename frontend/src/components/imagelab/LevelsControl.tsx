import { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";

import { autoLevels, DEFAULT_LEVELS, type LevelsValue } from "./levels";

function Slider({ label, value, min, max, step, onChange }: {
  label: string; value: number; min: number; max: number; step: number; onChange: (v: number) => void;
}) {
  return (
    <label className="grid grid-cols-[7rem_1fr_3.5rem] items-center gap-2 text-xs text-gray-600 dark:text-gray-400">
      <span>{label}</span>
      <input type="range" min={min} max={max} step={step} value={value} onChange={(e) => onChange(Number(e.target.value))} className="accent-brand-500" />
      <input
        type="number" min={min} max={max} step={step} value={value}
        onChange={(e) => e.target.value !== "" && onChange(Math.min(max, Math.max(min, Number(e.target.value))))}
        className="w-full rounded border border-gray-300 bg-transparent px-1 py-0.5 text-right text-xs dark:border-gray-700 dark:text-white/90"
      />
    </label>
  );
}

/** Photoshop-style levels over the histogram of the image as levels receive it. */
export default function LevelsControl({ histogram, value, onChange }: {
  histogram?: number[]; value: LevelsValue; onChange: (v: LevelsValue) => void;
}) {
  const { t } = useTranslation();
  const canvasRef = useRef<HTMLCanvasElement>(null);

  useEffect(() => {
    const canvas = canvasRef.current;
    const ctx = canvas?.getContext("2d");
    if (!canvas || !ctx) return;
    const { width, height } = canvas;
    ctx.clearRect(0, 0, width, height);
    if (histogram) {
      // Square root scale: a dominant background bin would flatten everything else.
      const peak = Math.sqrt(Math.max(1, ...histogram));
      ctx.fillStyle = "#6b7280";
      for (let i = 0; i < 256; i++) {
        const h = (Math.sqrt(histogram[i]) / peak) * (height - 2);
        ctx.fillRect(i, height - h, 1, h);
      }
    }
    const mark = (x: number, color: string) => {
      ctx.fillStyle = color;
      ctx.fillRect(Math.round(x) - 1, 0, 2, height);
    };
    mark(value.inBlack, "#111827");
    mark(value.inWhite, "#e5e7eb");
    // The gamma handle sits where input maps to mid-grey: inBlack + span * 0.5^gamma.
    mark(value.inBlack + (value.inWhite - value.inBlack) * Math.pow(0.5, value.gamma), "#3b82f6");
  }, [histogram, value]);

  const set = (patch: Partial<LevelsValue>) => onChange({ ...value, ...patch });

  return (
    <div className="flex flex-col gap-2">
      <canvas
        ref={canvasRef}
        width={256}
        height={72}
        className="h-[72px] w-full rounded border border-gray-200 bg-white [image-rendering:pixelated] dark:border-gray-700 dark:bg-gray-800"
      />
      <Slider label={t("imageLab.levels.inBlack")} value={value.inBlack} min={0} max={value.inWhite - 1} step={1} onChange={(v) => set({ inBlack: v })} />
      <Slider label={t("imageLab.levels.gamma")} value={value.gamma} min={0.1} max={9.99} step={0.01} onChange={(v) => set({ gamma: v })} />
      <Slider label={t("imageLab.levels.inWhite")} value={value.inWhite} min={value.inBlack + 1} max={255} step={1} onChange={(v) => set({ inWhite: v })} />
      <Slider label={t("imageLab.levels.outBlack")} value={value.outBlack} min={0} max={value.outWhite} step={1} onChange={(v) => set({ outBlack: v })} />
      <Slider label={t("imageLab.levels.outWhite")} value={value.outWhite} min={value.outBlack} max={255} step={1} onChange={(v) => set({ outWhite: v })} />
      <div className="flex gap-3">
        <button type="button" disabled={!histogram} onClick={() => histogram && onChange(autoLevels(histogram))} className="text-xs text-brand-500 hover:underline disabled:opacity-40">
          {t("imageLab.levels.auto")}
        </button>
        <button type="button" onClick={() => onChange(DEFAULT_LEVELS)} className="text-xs text-gray-500 hover:underline">
          {t("imageLab.levels.reset")}
        </button>
      </div>
    </div>
  );
}
