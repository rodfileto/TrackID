import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { Modal } from "../ui/modal";
import Badge from "../ui/badge/Badge";
import {
  createCodificationVersion,
  deleteCodificationVersion,
  getCodificationVersionImageUrl,
  listCodificationVersions,
  previewCodificationVersion,
  setRecognitionVersion,
  type CodificationVersion,
  type FaceRecipe,
  type RenderedVersion,
} from "../../services/codificationVersions";

const buttonClass =
  "rounded-lg border border-gray-300 px-3 py-1.5 text-sm text-gray-700 hover:bg-gray-50 disabled:opacity-50 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-white/5";
const primaryClass =
  "rounded-lg bg-brand-500 px-3 py-1.5 text-sm font-medium text-white hover:bg-brand-600 disabled:opacity-50";
const smallClass = "text-xs font-medium text-brand-500 hover:text-brand-600 disabled:opacity-50";

/** The editor's controls, turned into a FaceRecipe (only what differs from neutral goes in). */
interface Controls {
  mirror: boolean;
  rotate: number;
  scale: number;
  grey: boolean;
  brightness: number;
  contrast: number;
  gamma: number;
}

const NEUTRAL: Controls = { mirror: false, rotate: 0, scale: 1, grey: false, brightness: 0, contrast: 0, gamma: 1 };

function toRecipe(c: Controls): FaceRecipe {
  const r: FaceRecipe = { rotate: c.rotate, mirror: c.mirror, scale: c.scale };
  if (c.grey) r.grey = "luminance";
  if (c.brightness !== 0 || c.contrast !== 0) r.brightnessContrast = { brightness: c.brightness, contrast: c.contrast };
  if (c.gamma !== 1) r.levels = { inBlack: 0, inWhite: 255, gamma: c.gamma, outBlack: 0, outWhite: 255 };
  return r;
}

function fromRecipe(r: FaceRecipe | undefined): Controls {
  if (!r) return NEUTRAL;
  return {
    mirror: !!r.mirror,
    rotate: r.rotate ?? 0,
    scale: r.scale ?? 1,
    grey: !!r.grey,
    brightness: r.brightnessContrast?.brightness ?? 0,
    contrast: r.brightnessContrast?.contrast ?? 0,
    gamma: r.levels?.gamma ?? 1,
  };
}

/** A short description of what a recipe does, for a version without a label. */
function useDescribe() {
  const { t } = useTranslation();
  return useCallback(
    (r: FaceRecipe | undefined) => {
      if (!r) return t("versions.original");
      const c = fromRecipe(r);
      const parts: string[] = [];
      if (c.mirror) parts.push(t("versions.mirror"));
      if (c.rotate) parts.push(`${t("versions.rotate")} ${c.rotate}°`);
      if (c.scale !== 1) parts.push(`${t("versions.scale")} ×${c.scale}`);
      if (c.grey) parts.push(t("versions.grey"));
      if (c.brightness) parts.push(`${t("versions.brightness")} ${c.brightness > 0 ? "+" : ""}${c.brightness}`);
      if (c.contrast) parts.push(`${t("versions.contrast")} ${c.contrast > 0 ? "+" : ""}${c.contrast}`);
      if (c.gamma !== 1) parts.push(`${t("versions.gamma")} ${c.gamma}`);
      return parts.length ? parts.join(" · ") : t("versions.noChange");
    },
    [t],
  );
}

function Slider({
  label,
  value,
  min,
  max,
  step,
  onChange,
  format,
}: {
  label: string;
  value: number;
  min: number;
  max: number;
  step: number;
  onChange: (v: number) => void;
  format?: (v: number) => string;
}) {
  return (
    <label className="block text-sm text-gray-700 dark:text-gray-300">
      <span className="flex justify-between">
        <span>{label}</span>
        <span className="font-mono text-theme-xs text-gray-500">{format ? format(value) : value}</span>
      </span>
      <input type="range" className="w-full" min={min} max={max} step={step} value={value} onChange={(e) => onChange(Number(e.target.value))} />
    </label>
  );
}

/** The versions of one face: render the face with a recipe (mirror, rotation, scale, grey,
 * brightness, contrast, gamma), save it as a new codification with its own embedding, and choose
 * the version the automatic matcher uses. Every version keeps its recipe, so the record says what
 * was done to the image. */
export default function CodificationVersionsModal({
  caseId,
  evidenceId,
  traceId,
  label,
  canWrite,
  onClose,
  onChanged,
}: {
  caseId: string;
  evidenceId: number;
  traceId: number;
  /** The face's label, e.g. "2-1". */
  label: string;
  canWrite: boolean;
  onClose: () => void;
  onChanged: () => void;
}) {
  const { t } = useTranslation();
  const describe = useDescribe();
  const [versions, setVersions] = useState<CodificationVersion[]>([]);
  const [images, setImages] = useState<Record<number, string>>({});
  const [controls, setControls] = useState<Controls>(NEUTRAL);
  const [preview, setPreview] = useState<RenderedVersion | null>(null);
  const [rendering, setRendering] = useState(false);
  const [newLabel, setNewLabel] = useState("");
  const [busy, setBusy] = useState(false);
  const [showRecord, setShowRecord] = useState(false);
  const recipe = useMemo(() => toRecipe(controls), [controls]);
  const set = (patch: Partial<Controls>) => setControls((c) => ({ ...c, ...patch }));

  const load = useCallback(async () => {
    try {
      const data = await listCodificationVersions(caseId, evidenceId, traceId);
      setVersions(data.versions);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : String(err));
    }
  }, [caseId, evidenceId, traceId]);
  useEffect(() => {
    void load();
  }, [load]);

  // The versions' own images; the original without one shows the neutral render.
  useEffect(() => {
    let cancelled = false;
    const created: string[] = [];
    (async () => {
      const out: Record<number, string> = {};
      for (const v of versions) {
        try {
          if (v.hasImage) {
            const url = await getCodificationVersionImageUrl(caseId, evidenceId, traceId, v.id);
            created.push(url);
            out[v.id] = url;
          } else {
            out[v.id] = (await previewCodificationVersion(caseId, evidenceId, traceId, toRecipe(NEUTRAL))).image;
          }
        } catch {
          // no thumbnail
        }
      }
      if (!cancelled) setImages(out);
    })();
    return () => {
      cancelled = true;
      created.forEach((u) => URL.revokeObjectURL(u));
    };
  }, [versions, caseId, evidenceId, traceId]);

  // Render the recipe on the server as the controls change (debounced).
  useEffect(() => {
    const controller = new AbortController();
    const timer = window.setTimeout(() => {
      setRendering(true);
      previewCodificationVersion(caseId, evidenceId, traceId, recipe, controller.signal)
        .then(setPreview)
        .catch((err) => {
          if (!controller.signal.aborted) toast.error(err instanceof Error ? err.message : String(err));
        })
        .finally(() => !controller.signal.aborted && setRendering(false));
    }, 250);
    return () => {
      window.clearTimeout(timer);
      controller.abort();
    };
  }, [caseId, evidenceId, traceId, recipe]);

  async function save() {
    setBusy(true);
    try {
      await createCodificationVersion(caseId, evidenceId, traceId, recipe, newLabel.trim());
      toast.success(t("versions.saved"));
      setNewLabel("");
      await load();
      onChanged();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  async function chooseForRecognition(v: CodificationVersion) {
    setBusy(true);
    try {
      await setRecognitionVersion(caseId, evidenceId, traceId, v.id);
      toast.success(t("versions.chosen", { name: v.label || describe(v.recipe) }));
      await load();
      onChanged();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  async function remove(v: CodificationVersion) {
    if (!window.confirm(t("versions.confirmDelete", { name: v.label || describe(v.recipe) }))) return;
    setBusy(true);
    try {
      await deleteCodificationVersion(caseId, evidenceId, traceId, v.id);
      await load();
      onChanged();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  const unchanged = JSON.stringify(controls) === JSON.stringify(NEUTRAL);

  return (
    <Modal isOpen onClose={onClose} className="m-4 max-w-6xl">
      <div className="p-5 sm:p-6">
        <h3 className="text-lg font-semibold text-gray-800 dark:text-white/90">{t("versions.title", { label })}</h3>
        <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">{t("versions.desc")}</p>

        <div className="mt-5 grid gap-6 lg:grid-cols-[1fr_280px_300px]">
          {/* Preview */}
          <div>
            <div className="relative flex min-h-64 items-center justify-center rounded-lg bg-gray-100 dark:bg-gray-800">
              {preview && <img src={preview.image} alt={t("versions.preview")} className="max-h-[55vh] max-w-full object-contain" />}
              {rendering && <span className="absolute right-2 top-2 rounded bg-black/60 px-2 py-0.5 text-theme-xs text-white">{t("versions.rendering")}</span>}
            </div>
            {preview && (
              <p className="mt-2 font-mono text-theme-xs text-gray-500">
                {preview.width}×{preview.height} · SHA-256 {preview.sha256.slice(0, 16)}…
              </p>
            )}
            {preview && (
              <button type="button" className={`mt-1 ${smallClass}`} onClick={() => setShowRecord((s) => !s)}>
                {showRecord ? t("versions.hideRecord") : t("versions.showRecord")}
              </button>
            )}
            {showRecord && preview && (
              <pre className="mt-2 max-h-48 overflow-auto whitespace-pre-wrap rounded bg-gray-50 p-2 text-theme-xs text-gray-700 dark:bg-white/[0.03] dark:text-gray-300">
                {preview.record}
              </pre>
            )}
          </div>

          {/* Controls */}
          <div className="space-y-3">
            <div className="flex flex-wrap gap-2">
              <button type="button" className={buttonClass} onClick={() => set({ mirror: !controls.mirror })} aria-pressed={controls.mirror}>
                {controls.mirror ? "✓ " : ""}
                {t("versions.mirror")}
              </button>
              <button type="button" className={buttonClass} onClick={() => set({ rotate: ((controls.rotate - 90 + 540) % 360) - 180 })}>
                ↺ 90°
              </button>
              <button type="button" className={buttonClass} onClick={() => set({ rotate: ((controls.rotate + 90 + 540) % 360) - 180 })}>
                ↻ 90°
              </button>
            </div>
            <Slider label={t("versions.rotate")} value={controls.rotate} min={-180} max={180} step={1} onChange={(rotate) => set({ rotate })} format={(v) => `${v}°`} />
            <Slider
              label={t("versions.scale")}
              value={controls.scale}
              min={0.1}
              max={4}
              step={0.05}
              onChange={(scale) => set({ scale })}
              format={(v) => `×${v.toFixed(2)}`}
            />
            <p className="-mt-2 text-theme-xs text-gray-500">{t("versions.scaleHint")}</p>
            <Slider label={t("versions.brightness")} value={controls.brightness} min={-100} max={100} step={1} onChange={(brightness) => set({ brightness })} />
            <Slider label={t("versions.contrast")} value={controls.contrast} min={-100} max={99} step={1} onChange={(contrast) => set({ contrast })} />
            <Slider label={t("versions.gamma")} value={controls.gamma} min={0.2} max={5} step={0.05} onChange={(gamma) => set({ gamma })} format={(v) => v.toFixed(2)} />
            <label className="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
              <input type="checkbox" checked={controls.grey} onChange={(e) => set({ grey: e.target.checked })} />
              {t("versions.grey")}
            </label>
            <button type="button" className={buttonClass} disabled={unchanged} onClick={() => setControls(NEUTRAL)}>
              {t("versions.reset")}
            </button>
            {canWrite && (
              <div className="space-y-2 border-t border-gray-200 pt-3 dark:border-gray-800">
                <input
                  type="text"
                  maxLength={200}
                  value={newLabel}
                  onChange={(e) => setNewLabel(e.target.value)}
                  placeholder={describe(recipe)}
                  className="w-full rounded-lg border border-gray-300 bg-transparent px-2 py-1.5 text-sm text-gray-800 dark:border-gray-700 dark:text-white/90"
                  aria-label={t("versions.label")}
                />
                <button type="button" className={`w-full ${primaryClass}`} disabled={busy || unchanged} onClick={() => void save()}>
                  {t("versions.save")}
                </button>
                <p className="text-theme-xs text-gray-500">{t("versions.saveHint")}</p>
              </div>
            )}
          </div>

          {/* Versions */}
          <div>
            <h4 className="mb-2 text-sm font-semibold text-gray-800 dark:text-white/90">{t("versions.list")}</h4>
            <ul className="max-h-[60vh] space-y-2 overflow-y-auto">
              {versions.map((v) => (
                <li
                  key={v.id}
                  className={`flex gap-3 rounded-lg border p-2 ${v.forRecognition ? "border-brand-500" : "border-gray-200 dark:border-gray-800"}`}
                >
                  {images[v.id] ? (
                    <img src={images[v.id]} alt="" className="h-16 w-16 shrink-0 rounded object-cover" />
                  ) : (
                    <span className="h-16 w-16 shrink-0 rounded bg-gray-100 dark:bg-gray-800" />
                  )}
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-medium text-gray-800 dark:text-white/90">
                      {label}-{v.sequence} · {v.label || describe(v.recipe)}
                    </p>
                    {v.label && v.recipe && <p className="truncate text-theme-xs text-gray-500">{describe(v.recipe)}</p>}
                    <div className="mt-1 flex flex-wrap items-center gap-1">
                      {v.forRecognition && (
                        <Badge size="sm" color="primary">
                          {t("versions.forRecognition")}
                        </Badge>
                      )}
                      {!v.hasEmbedding && (
                        <Badge size="sm" color="light">
                          {t("versions.noEmbedding")}
                        </Badge>
                      )}
                    </div>
                    <div className="mt-1 flex flex-wrap gap-3">
                      {v.recipe && (
                        <button type="button" className={smallClass} onClick={() => setControls(fromRecipe(v.recipe))}>
                          {t("versions.edit")}
                        </button>
                      )}
                      {canWrite && !v.forRecognition && (
                        <button type="button" className={smallClass} disabled={busy} onClick={() => void chooseForRecognition(v)}>
                          {t("versions.useForRecognition")}
                        </button>
                      )}
                      {canWrite && v.sequence > 1 && !v.forRecognition && (
                        <button type="button" className="text-xs font-medium text-error-500 hover:text-error-600 disabled:opacity-50" disabled={busy} onClick={() => void remove(v)}>
                          {t("common.delete")}
                        </button>
                      )}
                    </div>
                  </div>
                </li>
              ))}
            </ul>
            <p className="mt-3 text-theme-xs text-gray-500 dark:text-gray-400">{t("versions.recognitionHint")}</p>
          </div>
        </div>
      </div>
    </Modal>
  );
}
