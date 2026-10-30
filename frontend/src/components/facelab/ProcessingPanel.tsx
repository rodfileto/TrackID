import { useTranslation } from "react-i18next";

import LevelsControl from "../imagelab/LevelsControl";
import { DEFAULT_LEVELS } from "../imagelab/levels";
import type { FaceRecipe, FaceRender, Interpolation, Landmarks, Point, Rect } from "../../services/facelab";
import { IDENTITY_RECIPE } from "../../services/facelab";
import { Range, Section, Toggle } from "./controls";
import { buttonClass, inputClass } from "./style";

// The face recipe's steps, all global (FISWG Image Processing Techniques for Facial Image
// Comparison, §4.5 and §5.1): geometry, colour balance, grey, brightness/contrast, levels, curves.

const CURVE_INPUTS = [64, 128, 192];

export default function ProcessingPanel({ recipe, onChange, render, landmarks, face, otherIpd }: {
  recipe: FaceRecipe;
  onChange: (r: FaceRecipe) => void;
  render: FaceRender | null;
  landmarks?: Landmarks;
  face?: Rect;
  /** The other image's interpupillary distance as rendered, to bring this one to the same size. */
  otherIpd?: number;
}) {
  const { t } = useTranslation();
  const set = (patch: Partial<FaceRecipe>) => onChange({ ...recipe, ...patch });

  const eyes = landmarks?.points.slice(0, 2) as Point[] | undefined;
  const ipd = eyes ? Math.hypot(eyes[1][0] - eyes[0][0], eyes[1][1] - eyes[0][1]) : 0;
  const roll = eyes ? (Math.atan2(eyes[1][1] - eyes[0][1], eyes[1][0] - eyes[0][0]) * 180) / Math.PI : 0;

  const curve = recipe.curves?.points ?? CURVE_INPUTS.map((x) => [x, x] as Point);

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap gap-2">
        <button type="button" className={buttonClass} onClick={() => onChange({ ...IDENTITY_RECIPE })}>{t("faceLab.processing.reset")}</button>
        <span className="self-center text-sm text-gray-500 dark:text-gray-400">{t("faceLab.processing.minimum")}</span>
      </div>

      <Section title={t("faceLab.processing.geometry")}>
        <div className="flex flex-wrap items-center gap-2 text-sm">
          {face && (
            <button type="button" className={buttonClass} onClick={() => {
              const w = face.x2 - face.x1, h = face.y2 - face.y1;
              set({ crop: { x1: Math.round(face.x1 - w * 0.35), y1: Math.round(face.y1 - h * 0.35), x2: Math.round(face.x2 + w * 0.35), y2: Math.round(face.y2 + h * 0.25) } });
            }}>{t("faceLab.processing.cropToFace")}</button>
          )}
          {recipe.crop && <button type="button" className={buttonClass} onClick={() => set({ crop: undefined })}>{t("faceLab.processing.clearCrop")}</button>}
          {recipe.crop && <span className="text-gray-500 dark:text-gray-400">{Math.round(recipe.crop.x2 - recipe.crop.x1)} × {Math.round(recipe.crop.y2 - recipe.crop.y1)} px</span>}
        </div>
        <Range label={t("faceLab.processing.rotate")} value={recipe.rotate} min={-180} max={180} step={0.1} unit="°" onChange={(v) => set({ rotate: v })} />
        {eyes && (
          <button type="button" className={`${buttonClass} self-start`} onClick={() => set({ rotate: Math.round(-roll * 10) / 10 })}>
            {t("faceLab.processing.levelEyes", { roll: roll.toFixed(1) })}
          </button>
        )}
        <Toggle label={t("faceLab.processing.mirror")} checked={recipe.mirror} onChange={(v) => set({ mirror: v })} />
        <Range label={t("faceLab.processing.scale")} value={recipe.scale} min={0.1} max={8} step={0.01} unit="×" onChange={(v) => set({ scale: v })} />
        {ipd > 0 && (
          <p className="text-sm text-gray-500 dark:text-gray-400">
            {t("faceLab.processing.ipdNow", { ipd: (ipd * recipe.scale).toFixed(0) })}
            {otherIpd ? (
              <button type="button" className="ml-2 underline" onClick={() => set({ scale: Math.round((otherIpd / ipd) * 1000) / 1000 })}>
                {t("faceLab.processing.matchIpd", { ipd: otherIpd.toFixed(0) })}
              </button>
            ) : null}
          </p>
        )}
        {recipe.scale > 1 && <p className="text-sm text-warning-600">{t("faceLab.processing.enlarging")}</p>}
        <label className="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-400">
          {t("faceLab.processing.interpolation")}
          <select value={recipe.interpolation} onChange={(e) => set({ interpolation: e.target.value as Interpolation })} className={inputClass}>
            <option value="bicubic">{t("imageLab.interp.bicubic")}</option>
            <option value="bilinear">{t("imageLab.interp.bilinear")}</option>
            <option value="nearest">{t("imageLab.interp.nearest")}</option>
          </select>
        </label>
      </Section>

      <Section>
        <Toggle label={t("faceLab.processing.colorBalance")} checked={!!recipe.colorBalance}
          onChange={(v) => set({ colorBalance: v ? { red: 1, green: 1, blue: 1 } : undefined })} />
        {recipe.colorBalance && (["red", "green", "blue"] as const).map((c) => (
          <Range key={c} label={t(`imageLab.channels.${c}`)} value={recipe.colorBalance![c]} min={0.5} max={2} step={0.01} unit="×"
            onChange={(v) => set({ colorBalance: { ...recipe.colorBalance!, [c]: v } })} />
        ))}
      </Section>

      <Section>
        <label className="flex items-center gap-2 text-base text-gray-800 dark:text-white/90">
          {t("faceLab.processing.grey")}
          <select value={recipe.grey ?? ""} onChange={(e) => set({ grey: e.target.value as FaceRecipe["grey"] })} className={inputClass}>
            <option value="">{t("faceLab.processing.colour")}</option>
            {(["luminance", "red", "green", "blue"] as const).map((c) => <option key={c} value={c}>{t(`imageLab.channels.${c}`)}</option>)}
          </select>
        </label>
      </Section>

      <Section>
        <Toggle label={t("faceLab.processing.brightnessContrast")} checked={!!recipe.brightnessContrast}
          onChange={(v) => set({ brightnessContrast: v ? { brightness: 0, contrast: 0 } : undefined })} />
        {recipe.brightnessContrast && (
          <>
            <Range label={t("faceLab.processing.brightness")} value={recipe.brightnessContrast.brightness} min={-100} max={100} step={1}
              onChange={(v) => set({ brightnessContrast: { ...recipe.brightnessContrast!, brightness: v } })} />
            <Range label={t("faceLab.processing.contrast")} value={recipe.brightnessContrast.contrast} min={-100} max={99} step={1}
              onChange={(v) => set({ brightnessContrast: { ...recipe.brightnessContrast!, contrast: v } })} />
          </>
        )}
      </Section>

      <Section>
        <Toggle label={t("imageLab.levels.title")} checked={!!recipe.levels} onChange={(v) => set({ levels: v ? DEFAULT_LEVELS : undefined })} />
        {recipe.levels && <LevelsControl histogram={render?.histogram} value={recipe.levels} onChange={(l) => set({ levels: l })} />}
      </Section>

      <Section>
        <Toggle label={t("faceLab.processing.curves")} checked={!!recipe.curves}
          onChange={(v) => set({ curves: v ? { points: CURVE_INPUTS.map((x) => [x, x] as Point) } : undefined })} />
        {recipe.curves && curve.map(([x, y], i) => (
          <Range key={x} label={t("faceLab.processing.curvePoint", { input: x })} value={y} min={0} max={255} step={1}
            onChange={(v) => set({ curves: { points: curve.map((p, j) => (j === i ? [p[0], v] : p)) as Point[] } })} />
        ))}
      </Section>
    </div>
  );
}
