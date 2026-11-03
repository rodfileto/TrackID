import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import PageMeta from "../components/common/PageMeta";
import PageBreadcrumb from "../components/common/PageBreadCrumb";
import ComponentCard from "../components/common/ComponentCard";
import { ImageViewer } from "../components/ui/image-viewer/ImageViewer";
import OriginalCanvas, { type DrawMode } from "../components/imagelab/OriginalCanvas";
import LevelsControl from "../components/imagelab/LevelsControl";
import { DEFAULT_LEVELS, type LevelsValue } from "../components/imagelab/levels";
import {
  compareLab,
  estimateRidgePeriod,
  getLabInfo,
  renderLab,
  SourceExpiredError,
  uploadLabSource,
  type Channel,
  type Interpolation,
  type LabComparison,
  type LabInfo,
  type LabRender,
  type LabSource,
  type PpiMethod,
  type Recipe,
  type Rect,
} from "../services/imageproc";

// The image lab (docs/fingerprint-image-processing-plan.md): try the fingerprint processing
// steps on any image. The server renders every change and writes its record (in Portuguese, for
// the laudo); nothing is stored.

const inputClass =
  "rounded-lg border border-gray-300 bg-transparent px-2 py-1 text-sm text-gray-800 shadow-theme-xs focus:border-brand-300 focus:outline-hidden dark:border-gray-700 dark:bg-gray-900 dark:text-white/90";

interface Steps {
  channel: Channel;
  rotate: number;
  mirror: boolean;
  interpolation: Interpolation;
  flatten: number | null; // sigma in mm
  levels: LevelsValue | null;
  clahe: { tiles: number; clipLimit: number } | null;
  gabor: number | null; // orientations
  invert: boolean;
}

const DEFAULT_STEPS: Steps = {
  channel: "luminance",
  rotate: 0,
  mirror: false,
  interpolation: "bicubic",
  flatten: null,
  levels: null,
  clahe: null,
  gabor: null,
  invert: false,
};

const FINGER_PHOTO: Steps = {
  channel: "green",
  rotate: 0,
  mirror: true,
  interpolation: "bicubic",
  flatten: 2,
  levels: null,
  clahe: { tiles: 8, clipLimit: 2 },
  gabor: 16,
  invert: true,
};

function parseNumber(text: string): number {
  return Number(text.replace(",", "."));
}

function download(name: string, href: string) {
  const a = document.createElement("a");
  a.href = href;
  a.download = name;
  a.click();
}

function downloadText(name: string, text: string, type: string) {
  const url = URL.createObjectURL(new Blob([text], { type }));
  download(name, url);
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

function Toggle({ label, checked, onChange }: { label: string; checked: boolean; onChange: (v: boolean) => void }) {
  return (
    <label className="flex cursor-pointer items-center gap-2 text-sm font-medium text-gray-800 dark:text-white/90">
      <input type="checkbox" checked={checked} onChange={(e) => onChange(e.target.checked)} className="h-4 w-4 accent-brand-500" />
      {label}
    </label>
  );
}

function Range({ label, value, min, max, step, unit, onChange }: {
  label: string; value: number; min: number; max: number; step: number; unit?: string; onChange: (v: number) => void;
}) {
  return (
    <label className="grid grid-cols-[7rem_1fr_4.5rem] items-center gap-2 text-xs text-gray-600 dark:text-gray-400">
      <span>{label}</span>
      <input type="range" min={min} max={max} step={step} value={value} onChange={(e) => onChange(Number(e.target.value))} className="accent-brand-500" />
      <span className="flex items-center gap-1">
        <input
          type="number" min={min} max={max} step={step} value={value}
          onChange={(e) => e.target.value !== "" && onChange(Math.min(max, Math.max(min, Number(e.target.value))))}
          className="w-full rounded border border-gray-300 bg-transparent px-1 py-0.5 text-right text-xs dark:border-gray-700 dark:text-white/90"
        />
        {unit}
      </span>
    </label>
  );
}

function Section({ children }: { children: React.ReactNode }) {
  return <div className="flex flex-col gap-2 border-t border-gray-100 pt-3 first:border-t-0 first:pt-0 dark:border-gray-800">{children}</div>;
}

export default function ImageLab() {
  const { t } = useTranslation();
  const [info, setInfo] = useState<LabInfo | null>(null);
  const [file, setFile] = useState<File | null>(null);
  const [source, setSource] = useState<LabSource | null>(null);
  const [originalUrl, setOriginalUrl] = useState<string | null>(null);
  const [uploading, setUploading] = useState(false);

  const [mode, setMode] = useState<DrawMode>("none");
  const [crop, setCrop] = useState<Rect | undefined>();
  const [line, setLine] = useState<Rect | undefined>();
  const [lineMm, setLineMm] = useState("10");
  const [method, setMethod] = useState<PpiMethod>("ASSUMED");
  const [estimate, setEstimate] = useState<{ periodPx: number; confidence: number } | null>(null);
  const [estimating, setEstimating] = useState(false);
  const [ridgeMm, setRidgeMm] = useState("0.45");
  const [steps, setSteps] = useState<Steps>(DEFAULT_STEPS);

  const [render, setRender] = useState<LabRender | null>(null);
  const [rendering, setRendering] = useState(false);
  const [renderError, setRenderError] = useState("");

  const [reference, setReference] = useState<{ file: File; source: LabSource } | null>(null);
  const [referencePpi, setReferencePpi] = useState("500");
  const [comparison, setComparison] = useState<LabComparison | null>(null);
  const [comparing, setComparing] = useState(false);

  useEffect(() => {
    getLabInfo().then(setInfo).catch(() => setInfo(null));
  }, []);

  useEffect(() => () => {
    if (originalUrl) URL.revokeObjectURL(originalUrl);
  }, [originalUrl]);

  async function openFile(f: File) {
    setUploading(true);
    try {
      const s = await uploadLabSource(f);
      setFile(f);
      setSource(s);
      setOriginalUrl(URL.createObjectURL(f));
      setCrop(undefined);
      setLine(undefined);
      setEstimate(null);
      setRender(null);
      setComparison(null);
      setMethod(s.filePpi >= 300 ? "FILE" : "ASSUMED");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : String(err));
    } finally {
      setUploading(false);
    }
  }

  const set = (patch: Partial<Steps>) => setSteps((s) => ({ ...s, ...patch }));

  const calibrationPx = line ? Math.hypot(line.x2 - line.x1, line.y2 - line.y1) : 0;
  const ridgeDistance = parseNumber(ridgeMm);

  // The source resolution as the page computes it, for display; the server recomputes it.
  const sourcePpi = useMemo(() => {
    switch (method) {
      case "CALIBRATED":
        return line && parseNumber(lineMm) > 0 ? (calibrationPx / parseNumber(lineMm)) * 25.4 : null;
      case "FILE":
        return source?.filePpi || null;
      case "ESTIMATED":
        return estimate && ridgeDistance > 0 ? (estimate.periodPx / ridgeDistance) * 25.4 : null;
      default:
        return info?.outputPpi ?? 500;
    }
  }, [method, line, lineMm, calibrationPx, source, estimate, ridgeDistance, info]);

  const recipe = useMemo<Recipe | null>(() => {
    if (!source) return null;
    const src: Recipe["source"] = { method };
    if (method === "CALIBRATED") {
      if (!line || !(parseNumber(lineMm) > 0)) return null;
      src.calibration = { ...line, mm: parseNumber(lineMm) };
    } else if (method === "FILE") {
      if (!source.filePpi) return null;
      src.ppi = source.filePpi;
    } else if (method === "ESTIMATED") {
      if (!estimate) return null;
      src.ridgePeriodPx = estimate.periodPx;
      src.ridgeDistanceMm = ridgeDistance;
    }
    return {
      source: src,
      channel: steps.channel,
      crop,
      rotate: steps.rotate,
      mirror: steps.mirror,
      interpolation: steps.interpolation,
      flatten: steps.flatten != null ? { sigmaMm: steps.flatten } : undefined,
      levels: steps.levels ?? undefined,
      clahe: steps.clahe ?? undefined,
      gabor: steps.gabor != null ? { orientations: steps.gabor } : undefined,
      invert: steps.invert,
    };
  }, [source, method, line, lineMm, estimate, ridgeDistance, steps, crop]);

  const recipeKey = recipe ? JSON.stringify(recipe) : "";
  const renderAbort = useRef<AbortController | null>(null);

  // A score belongs to the recipe it was computed with.
  useEffect(() => setComparison(null), [recipeKey]);

  // Every change renders again, after a pause so dragging a slider sends one request.
  useEffect(() => {
    if (!source || !recipe || !file) return;
    const timer = setTimeout(async () => {
      renderAbort.current?.abort();
      const abort = new AbortController();
      renderAbort.current = abort;
      setRendering(true);
      try {
        let result: LabRender;
        try {
          result = await renderLab(source.id, recipe, abort.signal);
        } catch (err) {
          if (!(err instanceof SourceExpiredError)) throw err;
          await uploadLabSource(file);
          result = await renderLab(source.id, recipe, abort.signal);
        }
        setRender(result);
        setRenderError("");
      } catch (err) {
        if (abort.signal.aborted) return;
        setRenderError(err instanceof Error ? err.message : String(err));
      } finally {
        if (!abort.signal.aborted) setRendering(false);
      }
    }, 350);
    return () => clearTimeout(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [recipeKey]);

  const measureRidges = useCallback(async () => {
    if (!source) return;
    setEstimating(true);
    try {
      setEstimate(await estimateRidgePeriod(source.id, crop, steps.channel));
      setMethod("ESTIMATED");
    } catch (err) {
      if (err instanceof SourceExpiredError && file) {
        await uploadLabSource(file);
        setEstimate(await estimateRidgePeriod(source.id, crop, steps.channel));
        setMethod("ESTIMATED");
      } else {
        toast.error(err instanceof Error ? err.message : String(err));
      }
    } finally {
      setEstimating(false);
    }
  }, [source, crop, steps.channel, file]);

  function applyFingerPhoto() {
    setSteps({ ...FINGER_PHOTO, rotate: steps.rotate });
    if (method === "ASSUMED") void measureRidges();
  }

  async function openReference(f: File) {
    try {
      const s = await uploadLabSource(f);
      setReference({ file: f, source: s });
      if (s.filePpi >= 300) setReferencePpi(String(Math.round(s.filePpi)));
      setComparison(null);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : String(err));
    }
  }

  async function compare() {
    if (!source || !recipe || !reference) return;
    const ppi = parseNumber(referencePpi);
    const referenceRecipe: Recipe = {
      source: ppi === (info?.outputPpi ?? 500) ? { method: "ASSUMED" } : { method: "FILE", ppi },
      channel: "luminance",
      rotate: 0,
      mirror: false,
      interpolation: "bicubic",
      invert: false,
    };
    setComparing(true);
    try {
      let result: LabComparison;
      try {
        result = await compareLab(source.id, recipe, reference.source.id, referenceRecipe);
      } catch (err) {
        if (!(err instanceof SourceExpiredError) || !file) throw err;
        await uploadLabSource(file);
        await uploadLabSource(reference.file);
        result = await compareLab(source.id, recipe, reference.source.id, referenceRecipe);
      }
      setComparison(result);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : String(err));
    } finally {
      setComparing(false);
    }
  }

  const baseName = source?.filename.replace(/\.[^.]+$/, "") ?? "imagem";
  const lowConfidence = estimate != null && estimate.confidence < 4;

  return (
    <>
      <PageMeta title={`${t("imageLab.title")} | TrackID`} description={t("imageLab.desc")} />
      <PageBreadcrumb pageTitle={t("imageLab.title")} />
      <div className="flex flex-col gap-4">
        <ComponentCard title={t("imageLab.title")} desc={t("imageLab.desc")}>
          <div className="flex flex-wrap items-center gap-4">
            <label className="inline-flex h-10 cursor-pointer items-center rounded-lg bg-brand-500 px-4 text-sm font-medium text-white hover:bg-brand-600">
              {uploading ? t("imageLab.reading") : source ? t("imageLab.openAnother") : t("imageLab.open")}
              <input type="file" accept="image/*,.tif,.tiff,.bmp" className="hidden" onChange={(e) => e.target.files?.[0] && openFile(e.target.files[0])} />
            </label>
            {source && (
              <div className="text-xs text-gray-600 dark:text-gray-400">
                <div className="font-medium text-gray-800 dark:text-white/90">{source.filename}</div>
                <div>
                  {source.format.toUpperCase()} · {source.width} × {source.height} px
                  {source.orientation > 1 && ` · ${t("imageLab.exifApplied", { orientation: source.orientation })}`}
                  {source.filePpi > 0 && ` · ${t("imageLab.fileStates", { ppi: Math.round(source.filePpi) })}`}
                </div>
                <div className="font-mono">SHA-256 {source.sha256}</div>
              </div>
            )}
          </div>
        </ComponentCard>

        {source && originalUrl && (
          <div className="grid gap-4 xl:grid-cols-[1fr_1fr_22rem]">
            <ComponentCard title={t("imageLab.original")}>
              <div className="flex flex-wrap gap-2 text-xs">
                <button type="button" onClick={() => setMode(mode === "crop" ? "none" : "crop")}
                  className={`rounded-md border px-2 py-1 ${mode === "crop" ? "border-amber-500 bg-amber-50 text-amber-700 dark:bg-amber-500/10" : "border-gray-300 dark:border-gray-700 dark:text-gray-300"}`}>
                  {t("imageLab.drawCrop")}
                </button>
                {crop && (
                  <button type="button" onClick={() => setCrop(undefined)} className="rounded-md border border-gray-300 px-2 py-1 dark:border-gray-700 dark:text-gray-300">
                    {t("imageLab.clearCrop")}
                  </button>
                )}
                <button type="button" onClick={() => setMode(mode === "line" ? "none" : "line")}
                  className={`rounded-md border px-2 py-1 ${mode === "line" ? "border-red-500 bg-red-50 text-red-700 dark:bg-red-500/10" : "border-gray-300 dark:border-gray-700 dark:text-gray-300"}`}>
                  {t("imageLab.drawScale")}
                </button>
              </div>
              {/* Always there, so the canvas does not move when a drawing mode starts. */}
              <p className="min-h-4 text-xs text-gray-500 dark:text-gray-400">
                {mode !== "none" && t(mode === "crop" ? "imageLab.cropHint" : "imageLab.scaleHint")}
              </p>
              <OriginalCanvas
                src={originalUrl}
                crop={crop}
                line={line}
                mode={mode}
                onCrop={setCrop}
                onLine={(l) => {
                  setLine(l);
                  setMethod("CALIBRATED");
                }}
                onDone={() => setMode("none")}
                className="h-[520px] w-full rounded-lg"
              />
            </ComponentCard>

            <ComponentCard title={t("imageLab.result", { ppi: info?.outputPpi ?? 500 })}>
              <div className="flex min-h-5 flex-wrap gap-x-3 text-xs text-gray-500 dark:text-gray-400">
                {render && <span>{render.width} × {render.height} px · {t("imageLab.scale", { scale: render.scale.toFixed(3) })}</span>}
                {rendering && <span>{t("imageLab.rendering")}</span>}
              </div>
              {renderError && <p className="text-sm text-error-500">{renderError}</p>}
              {!recipe && <p className="text-sm text-gray-500 dark:text-gray-400">{t("imageLab.needResolution")}</p>}
              {render ? (
                <ImageViewer src={render.image} keepView showRotate={false} className="h-[520px] w-full rounded-lg" />
              ) : (
                <div className="h-[520px] w-full rounded-lg bg-gray-100 dark:bg-gray-900" />
              )}
            </ComponentCard>

            <ComponentCard title={t("imageLab.steps")}>
              <div className="flex flex-col gap-3">
                <div className="flex flex-wrap gap-2">
                  <button type="button" onClick={applyFingerPhoto} className="rounded-lg bg-brand-500 px-3 py-1.5 text-xs font-medium text-white hover:bg-brand-600">
                    {t("imageLab.fingerPhoto")}
                  </button>
                  <button type="button" onClick={() => setSteps(DEFAULT_STEPS)} className="rounded-lg border border-gray-300 px-3 py-1.5 text-xs dark:border-gray-700 dark:text-gray-300">
                    {t("imageLab.reset")}
                  </button>
                </div>

                <Section>
                  <span className="text-sm font-medium text-gray-800 dark:text-white/90">{t("imageLab.resolution.title")}</span>
                  {(["CALIBRATED", "FILE", "ESTIMATED", "ASSUMED"] as const).map((m) => (
                    <label key={m} className="flex items-start gap-2 text-xs text-gray-700 dark:text-gray-300">
                      <input type="radio" name="ppi-method" checked={method === m} onChange={() => setMethod(m)} disabled={m === "FILE" && !source.filePpi} className="mt-0.5 accent-brand-500" />
                      <span>{t(`imageLab.resolution.${m}`)}</span>
                    </label>
                  ))}
                  {method === "CALIBRATED" && (
                    <div className="flex flex-wrap items-center gap-2 text-xs text-gray-600 dark:text-gray-400">
                      {line ? t("imageLab.resolution.linePx", { px: calibrationPx.toFixed(1) }) : t("imageLab.resolution.noLine")}
                      <input value={lineMm} onChange={(e) => setLineMm(e.target.value)} className={`${inputClass} w-16 text-right`} /> mm
                    </div>
                  )}
                  {method === "FILE" && source.filePpi > 0 && source.filePpi < 300 && (
                    <p className="text-xs text-warning-600">{t("imageLab.resolution.fileSuspicious")}</p>
                  )}
                  {method === "ESTIMATED" && (
                    <div className="flex flex-col gap-1 text-xs text-gray-600 dark:text-gray-400">
                      <div className="flex flex-wrap items-center gap-2">
                        <button type="button" onClick={measureRidges} disabled={estimating} className="rounded-md border border-gray-300 px-2 py-1 dark:border-gray-700 dark:text-gray-300">
                          {estimating ? t("imageLab.resolution.measuring") : t("imageLab.resolution.measure")}
                        </button>
                        {estimate && <span>{t("imageLab.resolution.period", { px: estimate.periodPx.toFixed(1) })}</span>}
                      </div>
                      <div className="flex items-center gap-2">
                        {t("imageLab.resolution.ridgeDistance")}
                        <input value={ridgeMm} onChange={(e) => setRidgeMm(e.target.value)} className={`${inputClass} w-16 text-right`} /> mm
                      </div>
                      {lowConfidence && <p className="text-warning-600">{t("imageLab.resolution.lowConfidence")}</p>}
                      <p>{t(crop ? "imageLab.resolution.inCrop" : "imageLab.resolution.wholeImage")}</p>
                    </div>
                  )}
                  <p className="text-xs font-medium text-gray-800 dark:text-white/90">
                    {sourcePpi ? t("imageLab.resolution.summary", { from: Math.round(sourcePpi), to: info?.outputPpi ?? 500 }) : t("imageLab.needResolution")}
                  </p>
                </Section>

                <Section>
                  <span className="text-sm font-medium text-gray-800 dark:text-white/90">{t("imageLab.geometry")}</span>
                  <Range label={t("imageLab.rotate")} value={steps.rotate} min={-180} max={180} step={0.5} unit="°" onChange={(v) => set({ rotate: v })} />
                  <Toggle label={t("imageLab.mirror")} checked={steps.mirror} onChange={(v) => set({ mirror: v })} />
                  <label className="flex items-center gap-2 text-xs text-gray-600 dark:text-gray-400">
                    {t("imageLab.interpolation")}
                    <select value={steps.interpolation} onChange={(e) => set({ interpolation: e.target.value as Interpolation })} className={inputClass}>
                      <option value="bicubic">{t("imageLab.interp.bicubic")}</option>
                      <option value="bilinear">{t("imageLab.interp.bilinear")}</option>
                      <option value="nearest">{t("imageLab.interp.nearest")}</option>
                    </select>
                  </label>
                </Section>

                <Section>
                  <label className="flex items-center gap-2 text-sm font-medium text-gray-800 dark:text-white/90">
                    {t("imageLab.channel")}
                    <select value={steps.channel} onChange={(e) => set({ channel: e.target.value as Channel })} className={inputClass}>
                      {(["luminance", "red", "green", "blue"] as const).map((c) => (
                        <option key={c} value={c}>{t(`imageLab.channels.${c}`)}</option>
                      ))}
                    </select>
                  </label>
                </Section>

                <Section>
                  <Toggle label={t("imageLab.flatten")} checked={steps.flatten != null} onChange={(v) => set({ flatten: v ? 2 : null })} />
                  {steps.flatten != null && (
                    <Range label={t("imageLab.flattenSigma")} value={steps.flatten} min={0.5} max={10} step={0.1} unit="mm" onChange={(v) => set({ flatten: v })} />
                  )}
                </Section>

                <Section>
                  <Toggle label={t("imageLab.levels.title")} checked={steps.levels != null} onChange={(v) => set({ levels: v ? DEFAULT_LEVELS : null })} />
                  {steps.levels && <LevelsControl histogram={render?.histogram} value={steps.levels} onChange={(l) => set({ levels: l })} />}
                </Section>

                <Section>
                  <Toggle label={t("imageLab.clahe")} checked={steps.clahe != null} onChange={(v) => set({ clahe: v ? { tiles: 8, clipLimit: 2 } : null })} />
                  {steps.clahe && (
                    <>
                      <Range label={t("imageLab.claheTiles")} value={steps.clahe.tiles} min={2} max={16} step={1} onChange={(v) => set({ clahe: { ...steps.clahe!, tiles: v } })} />
                      <Range label={t("imageLab.claheClip")} value={steps.clahe.clipLimit} min={1} max={10} step={0.1} onChange={(v) => set({ clahe: { ...steps.clahe!, clipLimit: v } })} />
                    </>
                  )}
                </Section>

                <Section>
                  <Toggle label={t("imageLab.gabor")} checked={steps.gabor != null} onChange={(v) => set({ gabor: v ? 16 : null })} />
                  {steps.gabor != null && (
                    <>
                      <Range label={t("imageLab.gaborOrientations")} value={steps.gabor} min={4} max={36} step={1} onChange={(v) => set({ gabor: v })} />
                      {render && render.gaborPeriodPx > 0 && (
                        <p className="text-xs text-gray-500 dark:text-gray-400">{t("imageLab.gaborPeriod", { px: render.gaborPeriodPx.toFixed(1) })}</p>
                      )}
                      <p className="text-xs text-warning-600">{t("imageLab.gaborWarning")}</p>
                    </>
                  )}
                </Section>

                <Section>
                  <Toggle label={t("imageLab.invert")} checked={steps.invert} onChange={(v) => set({ invert: v })} />
                </Section>
              </div>
            </ComponentCard>
          </div>
        )}

        {render && (
          <ComponentCard title={t("imageLab.record")} desc={t("imageLab.recordDesc")}>
            <div className="flex flex-wrap gap-2">
              <button type="button" onClick={() => navigator.clipboard?.writeText(render.record).then(() => toast.success(t("imageLab.copied")))}
                className="rounded-lg border border-gray-300 px-3 py-1.5 text-xs dark:border-gray-700 dark:text-gray-300">{t("imageLab.copy")}</button>
              <button type="button" onClick={() => download(`${baseName}-500ppi.png`, render.image)}
                className="rounded-lg border border-gray-300 px-3 py-1.5 text-xs dark:border-gray-700 dark:text-gray-300">{t("imageLab.downloadPng")}</button>
              <button type="button" onClick={() => downloadText(`${baseName}-receita.json`, JSON.stringify({ original: { filename: source?.filename, sha256: source?.sha256 }, recipe: render.recipe, result: { sha256: render.sha256, width: render.width, height: render.height, ppi: info?.outputPpi ?? 500 }, software: info?.software }, null, 2), "application/json")}
                className="rounded-lg border border-gray-300 px-3 py-1.5 text-xs dark:border-gray-700 dark:text-gray-300">{t("imageLab.downloadRecipe")}</button>
              <button type="button" onClick={() => downloadText(`${baseName}-registro.txt`, render.record, "text/plain")}
                className="rounded-lg border border-gray-300 px-3 py-1.5 text-xs dark:border-gray-700 dark:text-gray-300">{t("imageLab.downloadRecord")}</button>
            </div>
            <pre className={`max-h-[32rem] overflow-auto whitespace-pre-wrap rounded-lg bg-gray-50 p-4 [font-family:inherit] text-sm leading-relaxed text-gray-800 dark:bg-gray-900 dark:text-gray-200 ${rendering ? "opacity-60" : ""}`}>
              {render.record}
            </pre>
          </ComponentCard>
        )}

        {render && info?.compare && (
          <ComponentCard title={t("imageLab.compare.title")} desc={t("imageLab.compare.desc", { threshold: info.matchThreshold })}>
            <div className="flex flex-wrap items-center gap-3 text-sm">
              <label className="inline-flex h-9 cursor-pointer items-center rounded-lg border border-gray-300 px-3 text-xs dark:border-gray-700 dark:text-gray-300">
                {reference ? reference.source.filename : t("imageLab.compare.open")}
                <input type="file" accept="image/*,.tif,.tiff,.bmp" className="hidden" onChange={(e) => e.target.files?.[0] && openReference(e.target.files[0])} />
              </label>
              <label className="flex items-center gap-2 text-xs text-gray-600 dark:text-gray-400">
                {t("imageLab.compare.ppi")}
                <input value={referencePpi} onChange={(e) => setReferencePpi(e.target.value)} className={`${inputClass} w-20 text-right`} />
              </label>
              <button type="button" disabled={!reference || comparing} onClick={compare} className="rounded-lg bg-brand-500 px-4 py-2 text-xs font-medium text-white hover:bg-brand-600 disabled:opacity-50">
                {comparing ? t("imageLab.compare.running") : t("imageLab.compare.run")}
              </button>
              {comparison && (
                <span className={`font-medium ${comparison.score >= comparison.threshold ? "text-success-600" : "text-gray-700 dark:text-gray-300"}`}>
                  {t("imageLab.compare.score", { score: comparison.score.toFixed(1) })}{" "}
                  {comparison.score >= comparison.threshold ? t("imageLab.compare.above") : t("imageLab.compare.below")}
                </span>
              )}
            </div>
            {comparison && (
              <div className="grid gap-4 md:grid-cols-2">
                <ImageViewer src={render.image} className="h-80 w-full rounded-lg" showRotate={false} />
                <ImageViewer src={comparison.referenceImage} className="h-80 w-full rounded-lg" showRotate={false} />
              </div>
            )}
          </ComponentCard>
        )}
      </div>
    </>
  );
}
