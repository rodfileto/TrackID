import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import PageMeta from "../components/common/PageMeta";
import PageBreadcrumb from "../components/common/PageBreadCrumb";
import ComponentCard from "../components/common/ComponentCard";
import FaceStage, { MARKUP_COLOUR, type Guide, type StageImage, type Tool } from "../components/facelab/FaceStage";
import AnalysisPanel from "../components/facelab/AnalysisPanel";
import ProcessingPanel from "../components/facelab/ProcessingPanel";
import FindingsPanel from "../components/facelab/FindingsPanel";
import EvaluationPanel from "../components/facelab/EvaluationPanel";
import { Toggle } from "../components/facelab/controls";
import { buttonClass, primaryClass, useLabel } from "../components/facelab/style";
import { apply, IDENTITY, type View } from "../components/facelab/geometry";
import {
  alignFaces,
  checkSession,
  getFaceLabInfo,
  getFaceSource,
  IDENTITY_RECIPE,
  measureFace,
  renderFace,
  sheetFor,
  SourceExpiredError,
  uploadFaceSource,
  type FaceLabInfo,
  type FaceRender,
  type FaceSource,
  type ImageCode,
  type Landmarks,
  type Markup,
  type Metrics,
  type Point,
  type Session,
  type SheetResult,
  type Side,
  type Similarity,
  type Summary,
  type Warning,
} from "../services/facelab";

// The face lab (docs/facial-comparison-tool-plan.md): a one-to-one facial comparison after FISWG
// on any two images, questioned (Q) and known (K). Processing, alignment, markups, findings and
// the opinion are kept in the page as a session, which can be saved to a file; the server renders,
// checks and draws the sheet, and stores nothing. No matcher score is shown: the comparison is the
// examiner's morphological analysis.

const STAGE_HEIGHT = 560;

interface SideState {
  file: File | null;
  source: FaceSource | null;
  render: FaceRender | null;
  rendering: boolean;
  error: string;
  metrics: Metrics | null;
  /** A loaded session's image no longer in the server's memory: open the same file again. */
  missing?: { filename: string; sha256: string };
}

const EMPTY_SIDE: SideState = { file: null, source: null, render: null, rendering: false, error: "", metrics: null };

function emptySessionSide(): Side {
  return { sourceId: "", recipe: { ...IDENTITY_RECIPE }, analysis: { factors: [], mirrored: false, manipulationSuspected: false, done: false } };
}

function newSession(): Session {
  return { version: 1, questioned: emptySessionSide(), known: emptySessionSide(), questionedFirst: true, superimposition: false, pairs: [], markups: [], findings: [], factors: [] };
}

type Mode = "side" | "overlay" | "wipe" | "flicker";
type Tab = "analysis" | "processing" | "comparison" | "evaluation" | "record";

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

/** Landmarks placed at a typical frontal layout in the middle of a w x h image, for the examiner
 * to move onto the face when no detector found it. */
function defaultLandmarks(w: number, h: number): Landmarks {
  const ipd = Math.min(w, h) * 0.25, cx = w / 2, cy = h * 0.42;
  return {
    source: "EXAMINER",
    points: [[cx - ipd / 2, cy], [cx + ipd / 2, cy], [cx, cy + ipd * 0.6], [cx - ipd * 0.4, cy + ipd], [cx + ipd * 0.4, cy + ipd]],
  };
}

/** Renders a side again when its recipe changes (after a pause, so a slider sends one request),
 * and measures it again when its face box or landmarks change. */
function useSideWork(
  code: ImageCode,
  state: SideState,
  side: Side,
  recipeKey: string,
  rerender: (code: ImageCode, sourceId: string, recipeJSON: string, file: File | null) => Promise<void>,
  setSide: (code: ImageCode, patch: Partial<SideState>) => void,
) {
  const { source, file } = state;
  const { sourceId, face, landmarks } = side;
  useEffect(() => {
    if (!source || !sourceId) return;
    const timer = setTimeout(() => void rerender(code, sourceId, recipeKey, file), 350);
    return () => clearTimeout(timer);
  }, [code, source, sourceId, recipeKey, file, rerender]);

  const measureKey = JSON.stringify([face, landmarks]);
  useEffect(() => {
    if (!source || !sourceId) return;
    const timer = setTimeout(() => {
      const [f, l] = JSON.parse(measureKey);
      measureFace(sourceId, f ?? undefined, l ?? undefined)
        .then((m) => setSide(code, { metrics: m }))
        .catch(() => setSide(code, { metrics: null }));
    }, 300);
    return () => clearTimeout(timer);
  }, [code, source, sourceId, measureKey, setSide]);
}

export default function FaceLab() {
  const { t } = useTranslation();
  const label = useLabel();
  const [info, setInfo] = useState<FaceLabInfo | null>(null);
  const [session, setSession] = useState<Session>(newSession);
  const [sides, setSides] = useState<Record<ImageCode, SideState>>({ Q: EMPTY_SIDE, K: EMPTY_SIDE });
  const [alignment, setAlignment] = useState<Similarity | null>(null);
  const [alignWarnings, setAlignWarnings] = useState<Warning[]>([]);
  const [views, setViews] = useState<Record<ImageCode, View | null>>({ Q: null, K: null });
  const [mode, setMode] = useState<Mode>("side");
  const [tool, setTool] = useState<Tool>("pan");
  const [target, setTarget] = useState<ImageCode>("K");
  const [selected, setSelected] = useState<number | null>(null);
  const [showLandmarks, setShowLandmarks] = useState(true);
  const [showPairs, setShowPairs] = useState(true);
  const [pendingPair, setPendingPair] = useState<Point | null>(null);
  const [guides, setGuides] = useState<Guide[]>([]);
  const [opacity, setOpacity] = useState(0.5);
  const [wipe, setWipe] = useState<number | null>(null);
  const [flickerK, setFlickerK] = useState(true);
  const [tab, setTab] = useState<Tab>("analysis");
  const [tabSide, setTabSide] = useState<ImageCode>("Q");
  const [warnings, setWarnings] = useState<Warning[]>([]);
  const [summary, setSummary] = useState<Summary | null>(null);
  const [sheet, setSheet] = useState<SheetResult | null>(null);
  const [drawingSheet, setDrawingSheet] = useState(false);

  useEffect(() => {
    getFaceLabInfo().then(setInfo).catch((err) => toast.error(err instanceof Error ? err.message : String(err)));
  }, []);

  const sideKey = (code: ImageCode): "questioned" | "known" => (code === "Q" ? "questioned" : "known");
  const sessionSide = (code: ImageCode) => session[sideKey(code)];
  const setSessionSide = useCallback((code: ImageCode, patch: Partial<Side>) => {
    setSession((s) => ({ ...s, [code === "Q" ? "questioned" : "known"]: { ...s[code === "Q" ? "questioned" : "known"], ...patch } }));
  }, []);
  const setSide = useCallback((code: ImageCode, patch: Partial<SideState>) => setSides((s) => ({ ...s, [code]: { ...s[code], ...patch } })), []);

  // The known image stays hidden until the questioned image's analysis is done (ACE-V §5.1.1).
  const knownHidden = session.questionedFirst && !session.questioned.analysis.done;

  // ---- Opening images --------------------------------------------------------------------------

  async function openFile(code: ImageCode, f: File) {
    try {
      const src = await uploadFaceSource(f);
      const missing = sides[code].missing;
      if (missing) {
        if (src.sha256 !== missing.sha256) {
          toast.error(t("faceLab.open.wrongFile", { filename: missing.filename }));
          return;
        }
        setSide(code, { file: f, source: src, missing: undefined });
        return;
      }
      const face = [...src.faces].sort((a, b) => (b.box.x2 - b.box.x1) * (b.box.y2 - b.box.y1) - (a.box.x2 - a.box.x1) * (a.box.y2 - a.box.y1))[0];
      setSide(code, { ...EMPTY_SIDE, file: f, source: src });
      setSessionSide(code, {
        sourceId: src.id,
        recipe: { ...IDENTITY_RECIPE },
        face: face?.box,
        landmarks: face ? { points: face.landmarks, source: "DETECTOR" } : undefined,
        analysis: { ...emptySessionSide().analysis, captureDate: src.exif.dateTimeOriginal },
      });
      setSession((s) => ({ ...s, pairs: [], markups: (s.markups ?? []).filter((m) => m.image !== code) }));
      setAlignment(null);
      setViews((v) => ({ ...v, [code]: null }));
      setSheet(null);
      if (code === "Q") setTabSide("Q");
      if (src.faces.length === 0) toast.info(t(info?.detection ? "faceLab.open.noFace" : "faceLab.open.noDetector"));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : String(err));
    }
  }

  function chooseFace(code: ImageCode, index: number) {
    const face = sides[code].source?.faces[index];
    if (!face) return;
    setSessionSide(code, { face: face.box, landmarks: { points: face.landmarks, source: "DETECTOR" } });
  }

  // ---- Rendering, metrics ----------------------------------------------------------------------

  const renderAbort = useRef<Record<ImageCode, AbortController | null>>({ Q: null, K: null });
  const recipeKeys = { Q: JSON.stringify(session.questioned.recipe), K: JSON.stringify(session.known.recipe) };

  const rerender = useCallback(async (code: ImageCode, sourceId: string, recipeJSON: string, file: File | null) => {
    renderAbort.current[code]?.abort();
    const abort = new AbortController();
    renderAbort.current[code] = abort;
    setSide(code, { rendering: true });
    const recipe = JSON.parse(recipeJSON);
    try {
      let r: FaceRender;
      try {
        r = await renderFace(sourceId, recipe, abort.signal);
      } catch (err) {
        if (!(err instanceof SourceExpiredError) || !file) throw err;
        await uploadFaceSource(file);
        r = await renderFace(sourceId, recipe, abort.signal);
      }
      setSide(code, { render: r, error: "", rendering: false });
    } catch (err) {
      if (abort.signal.aborted) return;
      setSide(code, { error: err instanceof Error ? err.message : String(err), rendering: false });
    }
  }, [setSide]);

  useSideWork("Q", sides.Q, session.questioned, recipeKeys.Q, rerender, setSide);
  useSideWork("K", sides.K, session.known, recipeKeys.K, rerender, setSide);


  // ---- Alignment -------------------------------------------------------------------------------

  const qT = sides.Q.render?.transform ?? IDENTITY;
  const kT = sides.K.render?.transform ?? IDENTITY;
  const pairsKey = JSON.stringify(session.pairs ?? []);
  useEffect(() => {
    const pairs = session.pairs ?? [];
    if (pairs.length < 2 || !sides.Q.render || !sides.K.render) {
      setAlignment(null);
      return;
    }
    const timer = setTimeout(() => {
      alignFaces(session.questioned, session.known, pairs)
        .then((r) => {
          setAlignment(r.alignment);
          setAlignWarnings(r.warnings);
        })
        .catch((err) => toast.error(err instanceof Error ? err.message : String(err)));
    }, 200);
    return () => clearTimeout(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pairsKey, sides.Q.render, sides.K.render, JSON.stringify(session.questioned.landmarks), JSON.stringify(session.known.landmarks)]);

  /** Pairs the eye centres, each image's eyes taken left to right as rendered, so a mirrored
   * image still pairs right eye with right eye. */
  function alignByEyes() {
    const ql = session.questioned.landmarks, kl = session.known.landmarks;
    if (!ql || !kl) {
      toast.error(t("faceLab.align.needLandmarks"));
      return;
    }
    const order = (l: Landmarks, tr: typeof qT) => {
      const [a, b] = [l.points[0], l.points[1]];
      return apply(tr, a)[0] <= apply(tr, b)[0] ? [a, b] : [b, a];
    };
    const [q0, q1] = order(ql, qT), [k0, k1] = order(kl, kT);
    setSession((s) => ({ ...s, pairs: [{ q: q0, k: k0 }, { q: q1, k: k1 }] }));
    setPendingPair(null);
  }

  function clearAlignment() {
    setSession((s) => ({ ...s, pairs: [] }));
    setPendingPair(null);
    if (mode !== "side") setMode("side");
  }

  function pairPoint(code: ImageCode, p: Point) {
    if (code === "Q") {
      setPendingPair(p);
    } else if (pendingPair) {
      setSession((s) => ({ ...s, pairs: [...(s.pairs ?? []), { q: pendingPair, k: p }] }));
      setPendingPair(null);
    } else {
      toast.info(t("faceLab.align.qFirst"));
    }
  }

  // ---- Markups ---------------------------------------------------------------------------------

  const markups = useMemo(() => session.markups ?? [], [session.markups]);
  /** The number a new markup on image code takes: one marked on the other image only, or the next. */
  function nextNumber(code: ImageCode) {
    const mine = new Set(markups.filter((m) => m.image === code).map((m) => m.number));
    const theirs = markups.filter((m) => m.image !== code).map((m) => m.number).filter((n) => !mine.has(n)).sort((a, b) => a - b);
    if (theirs.length) return theirs[0];
    return markups.reduce((n, m) => Math.max(n, m.number), 0) + 1;
  }

  function addMarkup(m: Omit<Markup, "number">) {
    if (m.image === "K" && knownHidden) return;
    setSession((s) => ({ ...s, markups: [...(s.markups ?? []), { ...m, number: nextNumber(m.image) }] }));
  }

  function removeMarkup(i: number) {
    setSession((s) => ({ ...s, markups: (s.markups ?? []).filter((_, j) => j !== i) }));
    setSelected(null);
  }

  function renumber(i: number, n: number) {
    if (!(n >= 1 && n <= 99)) return;
    setSession((s) => ({ ...s, markups: (s.markups ?? []).map((m, j) => (j === i ? { ...m, number: n } : m)) }));
  }

  useEffect(() => {
    function key(e: KeyboardEvent) {
      const el = e.target as HTMLElement;
      if (el.tagName === "INPUT" || el.tagName === "TEXTAREA" || el.tagName === "SELECT") return;
      if ((e.key === "Delete" || e.key === "Backspace") && selected != null) removeMarkup(selected);
      if (e.key === "Escape") {
        setTool("pan");
        setPendingPair(null);
      }
    }
    window.addEventListener("keydown", key);
    return () => window.removeEventListener("keydown", key);
  });

  // ---- Superimposition modes -------------------------------------------------------------------

  useEffect(() => {
    if (mode !== "flicker") return;
    const id = setInterval(() => setFlickerK((v) => !v), 700);
    return () => clearInterval(id);
  }, [mode]);

  function chooseMode(m: Mode) {
    if (m !== "side") {
      if (!alignment) {
        toast.info(t("faceLab.view.alignFirst"));
        return;
      }
      setSession((s) => ({ ...s, superimposition: true }));
    }
    // The wipe starts across the middle of the questioned image (frame pixels).
    setWipe(m === "wipe" ? (sides.Q.render?.width ?? 0) / 2 : null);
    // One stage or two: the view fits the new stage again.
    if ((m === "side") !== (mode === "side")) setViews({ Q: null, K: null });
    setMode(m);
  }

  // ---- Checks and the sheet ---------------------------------------------------------------------

  const sessionKey = JSON.stringify(session);
  useEffect(() => {
    if (!info || !session.questioned.sourceId) return;
    const abort = new AbortController();
    const timer = setTimeout(() => {
      checkSession(session, abort.signal)
        .then((r) => {
          setWarnings(r.warnings);
          setSummary(r.summary);
        })
        .catch(() => {});
    }, 600);
    return () => {
      clearTimeout(timer);
      abort.abort();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sessionKey, info]);

  async function drawSheet() {
    setDrawingSheet(true);
    try {
      setSheet(await sheetFor(session));
      setTab("record");
    } catch (err) {
      if (err instanceof SourceExpiredError) {
        for (const code of ["Q", "K"] as const) if (sides[code].file) await uploadFaceSource(sides[code].file!);
        setSheet(await sheetFor(session));
        setTab("record");
      } else {
        toast.error(err instanceof Error ? err.message : String(err));
      }
    } finally {
      setDrawingSheet(false);
    }
  }

  // ---- Saving and loading the session ----------------------------------------------------------

  function saveSession() {
    const payload = {
      session,
      sources: { Q: { filename: sides.Q.source?.filename, sha256: sides.Q.source?.sha256 }, K: { filename: sides.K.source?.filename, sha256: sides.K.source?.sha256 } },
      software: info?.software,
      savedAt: new Date().toISOString(),
    };
    downloadText("comparacao-facial.json", JSON.stringify(payload, null, 2), "application/json");
  }

  async function loadSession(f: File) {
    try {
      const payload = JSON.parse(await f.text()) as { session: Session; sources: Record<ImageCode, { filename: string; sha256: string }> };
      if (!payload.session || payload.session.version !== info?.sessionVersion) throw new Error(t("faceLab.session.badFile"));
      const next: Record<ImageCode, SideState> = { Q: { ...EMPTY_SIDE }, K: { ...EMPTY_SIDE } };
      for (const code of ["Q", "K"] as const) {
        const id = payload.session[sideKey(code)].sourceId;
        if (!id) continue;
        try {
          next[code] = { ...EMPTY_SIDE, source: await getFaceSource(id) };
        } catch {
          next[code] = { ...EMPTY_SIDE, missing: payload.sources?.[code] ?? { filename: "?", sha256: id } };
        }
      }
      // Sessions saved before 2026-10-30 have one explanation per finding.
      for (const f of payload.session.findings ?? []) {
        if (f.explanation) {
          f.explanations = [...new Set([...(f.explanations ?? []), f.explanation])];
          delete f.explanation;
        }
      }
      setSession(payload.session);
      setSides(next);
      setViews({ Q: null, K: null });
      setSheet(null);
      setMode("side");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : String(err));
    }
  }

  // ---- Stage composition -----------------------------------------------------------------------

  const stageImage = (code: ImageCode, toFrame = IDENTITY, extra: Partial<StageImage> = {}): StageImage | null => {
    const r = sides[code].render;
    if (!r) return null;
    return { code, src: r.image, width: r.width, height: r.height, toRender: r.transform, toFrame, ...extra };
  };
  const kFrame = alignment?.transform ?? IDENTITY;

  const landmarks = { Q: session.questioned.landmarks, K: session.known.landmarks };
  const commonStage = {
    tool,
    markups,
    selected,
    onSelect: setSelected,
    onAddMarkup: addMarkup,
    onMoveMarkup: (i: number, points: Point[]) => setSession((s) => ({ ...s, markups: (s.markups ?? []).map((m, j) => (j === i ? { ...m, points } : m)) })),
    landmarks,
    showLandmarks,
    onLandmark: (code: ImageCode, i: number, p: Point) => {
      const l = sessionSide(code).landmarks;
      if (!l) return;
      const points = l.points.map((q, j) => (j === i ? p : q)) as Landmarks["points"];
      setSessionSide(code, { landmarks: { points, source: "EXAMINER" } });
    },
    pairs: session.pairs ?? [],
    pendingPair,
    showPairs,
    onPairPoint: pairPoint,
    guides,
    onGuides: setGuides,
    onCrop: (code: ImageCode, r: { x1: number; y1: number; x2: number; y2: number }) => {
      // The crop is drawn on the processed image; it is recorded on the original.
      const src = sides[code].source;
      if (!src) return;
      const crop = { x1: Math.max(0, r.x1), y1: Math.max(0, r.y1), x2: Math.min(src.width, r.x2), y2: Math.min(src.height, r.y2) };
      setSessionSide(code, { recipe: { ...sessionSide(code).recipe, crop } });
      setViews((v) => ({ ...v, [code]: null }));
      setTool("pan");
    },
    wipe,
    onWipe: setWipe,
  };

  const linkedView = alignment != null; // the known image is drawn in the questioned image's frame
  const viewFor = (code: ImageCode) => (linkedView ? views.Q : views[code]);
  const setViewFor = (code: ImageCode) => (v: View) => setViews((s) => (linkedView ? { ...s, Q: v } : { ...s, [code]: v }));

  const numbers = useMemo(() => [...new Set(markups.map((m) => m.number))].sort((a, b) => a - b), [markups]);
  const qIpdRendered = useMemo(() => {
    const l = session.questioned.landmarks;
    return l ? Math.hypot(l.points[1][0] - l.points[0][0], l.points[1][1] - l.points[0][1]) * (session.questioned.recipe.scale || 1) : undefined;
  }, [session.questioned.landmarks, session.questioned.recipe.scale]);

  if (!info) {
    return (
      <>
        <PageMeta title={`${t("faceLab.title")} | TrackID`} description={t("faceLab.desc")} />
        <PageBreadcrumb pageTitle={t("faceLab.title")} />
      </>
    );
  }

  const toolButton = (tl: Tool, text: string, disabled = false) => (
    <button key={tl} type="button" disabled={disabled} onClick={() => setTool(tool === tl ? "pan" : tl)}
      className={`rounded-md border px-2 py-1 text-sm disabled:opacity-40 ${tool === tl ? "border-brand-500 bg-brand-50 text-brand-700 dark:bg-brand-500/15 dark:text-brand-300" : "border-gray-300 text-gray-700 dark:border-gray-700 dark:text-gray-300"}`}>
      {text}
    </button>
  );

  const bothLoaded = !!sides.Q.render && !!sides.K.render;

  const sidePicker = (code: ImageCode) => {
    const s = sides[code];
    const side = sessionSide(code);
    return (
      <div className="flex flex-col gap-1">
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-base font-semibold text-gray-800 dark:text-white/90">{t(`faceLab.image.${code}`)}</span>
          <label className={`${primaryClass} cursor-pointer`}>
            {s.missing ? t("faceLab.open.again", { filename: s.missing.filename }) : s.source ? t("faceLab.open.another") : t("faceLab.open.open")}
            <input type="file" accept="image/*,.tif,.tiff,.bmp" className="hidden" onChange={(e) => {
              if (e.target.files?.[0]) void openFile(code, e.target.files[0]);
              e.target.value = "";
            }} />
          </label>
          {s.source && s.source.faces.length > 1 && (
            <select className="rounded border border-gray-300 px-1 py-0.5 text-sm dark:border-gray-700 dark:bg-gray-900 dark:text-white/90"
              onChange={(e) => chooseFace(code, Number(e.target.value))}>
              {s.source.faces.map((f, i) => <option key={i} value={i}>{t("faceLab.open.face", { n: i + 1, score: f.score.toFixed(2) })}</option>)}
            </select>
          )}
        </div>
        {s.source && (
          <div className="text-sm text-gray-600 dark:text-gray-400">
            {s.source.filename} · {s.source.width} × {s.source.height} px
            {side.landmarks ? ` · ${t(side.landmarks.source === "DETECTOR" ? "faceLab.open.detected" : "faceLab.open.placed")}` : ` · ${t("faceLab.open.noLandmarks")}`}
            {!side.landmarks && (
              <button type="button" className="ml-2 underline" onClick={() => {
                setSessionSide(code, { landmarks: defaultLandmarks(s.source!.width, s.source!.height) });
                setShowLandmarks(true);
                setTool("landmarks");
              }}>{t("faceLab.open.placeLandmarks")}</button>
            )}
          </div>
        )}
        {s.error && <p className="text-sm text-error-500">{s.error}</p>}
      </div>
    );
  };

  const hiddenCover = (
    <div className="flex items-center justify-center rounded-lg bg-gray-100 p-6 text-center text-base text-gray-600 dark:bg-gray-900 dark:text-gray-400" style={{ height: STAGE_HEIGHT }}>
      {t("faceLab.view.hidden")}
    </div>
  );

  const emptyStage = <div className="rounded-lg bg-gray-100 dark:bg-gray-900" style={{ height: STAGE_HEIGHT }} />;

  return (
    <>
      <PageMeta title={`${t("faceLab.title")} | TrackID`} description={t("faceLab.desc")} />
      <PageBreadcrumb pageTitle={t("faceLab.title")} />
      <div className="flex flex-col gap-4">
        <ComponentCard title={t("faceLab.title")} desc={t("faceLab.desc")}>
          <div className="grid gap-4 md:grid-cols-2">
            {sidePicker("Q")}
            {sidePicker("K")}
          </div>
          <div className="flex flex-wrap items-center gap-3 border-t border-gray-100 pt-3 dark:border-gray-800">
            <Toggle label={t("faceLab.session.questionedFirst")} checked={session.questionedFirst} onChange={(v) => setSession((s) => ({ ...s, questionedFirst: v }))} />
            <span className="grow" />
            <button type="button" className={buttonClass} onClick={saveSession} disabled={!session.questioned.sourceId}>{t("faceLab.session.save")}</button>
            <label className={`${buttonClass} cursor-pointer`}>
              {t("faceLab.session.load")}
              <input type="file" accept="application/json,.json" className="hidden" onChange={(e) => {
                if (e.target.files?.[0]) void loadSession(e.target.files[0]);
                e.target.value = "";
              }} />
            </label>
            <button type="button" className={buttonClass} onClick={() => {
              setSession(newSession());
              setSides({ Q: EMPTY_SIDE, K: EMPTY_SIDE });
              setViews({ Q: null, K: null });
              setSheet(null);
              setMode("side");
            }}>{t("faceLab.session.new")}</button>
          </div>
        </ComponentCard>

        {(sides.Q.source || sides.K.source) && (
          <ComponentCard title={t("faceLab.view.title")}>
            <div className="flex flex-col gap-2">
              <div className="flex flex-wrap items-center gap-2">
                <span className="text-sm text-gray-500 dark:text-gray-400">{t("faceLab.view.mode")}</span>
                {(["side", "overlay", "wipe", "flicker"] as const).map((m) => (
                  <button key={m} type="button" disabled={m !== "side" && (!bothLoaded || knownHidden)} onClick={() => chooseMode(m)}
                    className={`rounded-md border px-2 py-1 text-sm disabled:opacity-40 ${mode === m ? "border-brand-500 bg-brand-50 text-brand-700 dark:bg-brand-500/15 dark:text-brand-300" : "border-gray-300 text-gray-700 dark:border-gray-700 dark:text-gray-300"}`}>
                    {t(`faceLab.view.${m}`)}
                  </button>
                ))}
                {mode === "overlay" && (
                  <label className="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-400">
                    {t("faceLab.view.opacity")}
                    <input type="range" min={0} max={1} step={0.05} value={opacity} onChange={(e) => setOpacity(Number(e.target.value))} className="accent-brand-500" />
                  </label>
                )}
                {mode !== "side" && (
                  <label className="flex items-center gap-1 text-sm text-gray-600 dark:text-gray-400">
                    {t("faceLab.view.drawOn")}
                    <select value={target} onChange={(e) => setTarget(e.target.value as ImageCode)} className="rounded border border-gray-300 px-1 dark:border-gray-700 dark:bg-gray-900">
                      <option value="Q">Q</option>
                      <option value="K">K</option>
                    </select>
                  </label>
                )}
                <span className="grow" />
              </div>

              <div className="flex flex-wrap items-center gap-2">
                <span className="text-sm text-gray-500 dark:text-gray-400">{t("faceLab.tools.title")}</span>
                {toolButton("pan", t("faceLab.tools.pan"))}
                {toolButton("select", t("faceLab.tools.select"))}
                {info.markupKinds.map((k) => toolButton(k.code as Tool, label(k)))}
                <span className="mx-1 h-5 w-px bg-gray-200 dark:bg-gray-700" />
                {toolButton("crop", t("faceLab.tools.crop"))}
                {toolButton("landmarks", t("faceLab.tools.landmarks"))}
                {toolButton("pair", t("faceLab.tools.pair"), !bothLoaded || knownHidden)}
                {toolButton("guideH", t("faceLab.tools.guideH"), !alignment)}
                {toolButton("guideV", t("faceLab.tools.guideV"), !alignment)}
                {guides.length > 0 && <button type="button" className="text-sm underline text-gray-500" onClick={() => setGuides([])}>{t("faceLab.tools.clearGuides")}</button>}
              </div>

              <div className="flex flex-wrap items-center gap-2 text-sm">
                <span className="text-gray-500 dark:text-gray-400">{t("faceLab.align.title")}</span>
                <button type="button" className={buttonClass} disabled={!bothLoaded || knownHidden} onClick={alignByEyes}>{t("faceLab.align.eyes")}</button>
                {(session.pairs ?? []).length > 0 && <button type="button" className={buttonClass} onClick={clearAlignment}>{t("faceLab.align.clear")}</button>}
                {alignment && (
                  <span className="text-gray-600 dark:text-gray-400">
                    {t("faceLab.align.result", { pairs: alignment.pairs, scale: alignment.scale.toFixed(3), rotation: alignment.rotation.toFixed(1), rms: alignment.rms.toFixed(1) })}
                  </span>
                )}
                {pendingPair && <span className="text-fuchsia-600">{t("faceLab.align.pending")}</span>}
                <span className="grow" />
                <Toggle label={<span className="text-sm">{t("faceLab.view.showLandmarks")}</span>} checked={showLandmarks} onChange={setShowLandmarks} />
                <Toggle label={<span className="text-sm">{t("faceLab.view.showPairs")}</span>} checked={showPairs} onChange={setShowPairs} />
              </div>
              {alignWarnings.length > 0 && mode !== "side" && (
                <ul className="text-sm text-warning-700 dark:text-warning-400">{alignWarnings.map((w, i) => <li key={i}>• {w.message}</li>)}</ul>
              )}
              <p className="min-h-4 text-sm text-gray-500 dark:text-gray-400">{t(`faceLab.hints.${tool}`, { defaultValue: "" })}</p>

              <div className="grid gap-3 xl:grid-cols-[1fr_14rem]">
                {mode === "side" ? (
                  <div className="grid gap-3 md:grid-cols-2">
                    {(["Q", "K"] as const).map((code) => {
                      if (code === "K" && knownHidden && sides.K.source) return <div key={code}>{hiddenCover}</div>;
                      const im = stageImage(code, code === "K" && alignment ? kFrame : IDENTITY);
                      const view = viewFor(code);
                      return (
                        <div key={code} className="flex flex-col gap-1">
                          <span className="text-sm font-medium text-gray-600 dark:text-gray-400">
                            {t(`faceLab.image.${code}`)}{sides[code].rendering && ` · ${t("faceLab.view.rendering")}`}
                          </span>
                          {im ? (
                            <FaceStage {...commonStage} images={[im]} view={view} onView={setViewFor(code)} target={code} />
                          ) : emptyStage}
                        </div>
                      );
                    })}
                  </div>
                ) : (
                  (() => {
                    const q = stageImage("Q");
                    const k = stageImage("K", kFrame, {
                      opacity: mode === "overlay" ? opacity : 1,
                      visible: mode === "flicker" ? flickerK : true,
                    });
                    const imgs = [q, k].filter(Boolean) as StageImage[];
                    return (
                      <FaceStage {...commonStage} images={imgs} view={views.Q} onView={setViewFor("Q")} target={target} />
                    );
                  })()
                )}

                <div className="flex flex-col gap-2">
                  <span className="text-sm font-medium text-gray-600 dark:text-gray-400">{t("faceLab.markups.title")}</span>
                  {markups.length === 0 && <p className="text-sm text-gray-500 dark:text-gray-400">{t("faceLab.markups.none")}</p>}
                  <ul className="flex max-h-[30rem] flex-col gap-1 overflow-auto">
                    {markups.map((m, i) => (
                      <li key={i} onClick={() => setSelected(i)}
                        className={`flex cursor-pointer items-center gap-2 rounded px-2 py-1 text-sm ${selected === i ? "bg-red-50 dark:bg-red-500/10" : "hover:bg-gray-50 dark:hover:bg-white/5"}`}>
                        <span className="inline-block h-3 w-3 rounded-full" style={{ background: MARKUP_COLOUR }} />
                        <input type="number" min={1} max={99} value={m.number} onChange={(e) => renumber(i, Number(e.target.value))}
                          className="w-12 rounded border border-gray-300 bg-transparent px-1 text-sm dark:border-gray-700 dark:text-white/90" />
                        <span className="text-gray-700 dark:text-gray-300">{m.image} · {label(info.markupKinds.find((k) => k.code === m.kind))}</span>
                        <button type="button" className="ml-auto text-error-500" onClick={(e) => { e.stopPropagation(); removeMarkup(i); }}>×</button>
                      </li>
                    ))}
                  </ul>
                  <p className="text-xs text-gray-500 dark:text-gray-400">{t("faceLab.markups.rules")}</p>
                </div>
              </div>
            </div>
          </ComponentCard>
        )}

        {sides.Q.source && (
          <ComponentCard title={t("faceLab.steps.title")}>
            <div className="flex flex-wrap gap-1 border-b border-gray-200 dark:border-gray-800">
              {(["analysis", "processing", "comparison", "evaluation", "record"] as const).map((tb) => (
                <button key={tb} type="button" onClick={() => setTab(tb)}
                  className={`-mb-px border-b-2 px-3 py-2 text-base ${tab === tb ? "border-brand-500 font-medium text-brand-600 dark:text-brand-400" : "border-transparent text-gray-600 dark:text-gray-400"}`}>
                  {t(`faceLab.steps.${tb}`)}
                </button>
              ))}
            </div>

            {(tab === "analysis" || tab === "processing") && (
              <div className="flex gap-2">
                {(["Q", "K"] as const).map((code) => (
                  <button key={code} type="button" disabled={code === "K" && (knownHidden || !sides.K.source)} onClick={() => setTabSide(code)}
                    className={`rounded-md border px-3 py-1 text-sm disabled:opacity-40 ${tabSide === code ? "border-brand-500 bg-brand-50 text-brand-700 dark:bg-brand-500/15 dark:text-brand-300" : "border-gray-300 text-gray-700 dark:border-gray-700 dark:text-gray-300"}`}>
                    {t(`faceLab.image.${code}`)}
                  </button>
                ))}
                {knownHidden && <span className="self-center text-sm text-gray-500 dark:text-gray-400">{t("faceLab.view.hiddenShort")}</span>}
              </div>
            )}

            {tab === "analysis" && sides[tabSide].source && (
              <AnalysisPanel info={info} source={sides[tabSide].source!} metrics={sides[tabSide].metrics}
                analysis={sessionSide(tabSide).analysis} onChange={(a) => setSessionSide(tabSide, { analysis: a })} />
            )}
            {tab === "processing" && sides[tabSide].source && (
              <div className="grid gap-4 lg:grid-cols-[1fr_1fr]">
                <ProcessingPanel recipe={sessionSide(tabSide).recipe} render={sides[tabSide].render}
                  landmarks={sessionSide(tabSide).landmarks} face={sessionSide(tabSide).face}
                  otherIpd={tabSide === "K" ? qIpdRendered : undefined}
                  onChange={(r) => setSessionSide(tabSide, { recipe: r })} />
                <pre className="max-h-[32rem] overflow-auto whitespace-pre-wrap rounded-lg bg-gray-50 p-3 [font-family:inherit] text-sm leading-relaxed text-gray-700 dark:bg-gray-900 dark:text-gray-300">
                  {sides[tabSide].render?.record}
                </pre>
              </div>
            )}
            {tab === "comparison" && (
              knownHidden || !sides.K.source ? <p className="text-base text-gray-500 dark:text-gray-400">{t("faceLab.findings.needBoth")}</p> : (
                <FindingsPanel info={info} findings={session.findings ?? []} factors={session.factors ?? []} numbers={numbers}
                  onFindings={(f) => setSession((s) => ({ ...s, findings: f }))} onFactors={(f) => setSession((s) => ({ ...s, factors: f }))} />
              )
            )}
            {tab === "evaluation" && (
              <EvaluationPanel info={info} summary={summary} warnings={warnings} opinion={session.opinion}
                onOpinion={(o) => setSession((s) => ({ ...s, opinion: o }))} />
            )}
            {tab === "record" && (
              <div className="flex flex-col gap-3">
                <div className="flex flex-wrap gap-2">
                  <button type="button" className={primaryClass} disabled={!bothLoaded || drawingSheet} onClick={drawSheet}>
                    {drawingSheet ? t("faceLab.record.drawing") : sheet ? t("faceLab.record.redraw") : t("faceLab.record.draw")}
                  </button>
                  {sheet && (
                    <>
                      <button type="button" className={buttonClass} onClick={() => download("folha-comparacao-marcada.png", sheet.annotated)}>{t("faceLab.record.downloadAnnotated")}</button>
                      <button type="button" className={buttonClass} onClick={() => download("folha-comparacao.png", sheet.clean)}>{t("faceLab.record.downloadClean")}</button>
                      <button type="button" className={buttonClass} onClick={() => downloadText("registro-comparacao-facial.txt", sheet.record, "text/plain")}>{t("faceLab.record.downloadRecord")}</button>
                      <button type="button" className={buttonClass} onClick={() => navigator.clipboard?.writeText(sheet.record).then(() => toast.success(t("imageLab.copied")))}>{t("imageLab.copy")}</button>
                    </>
                  )}
                </div>
                {!bothLoaded && <p className="text-base text-gray-500 dark:text-gray-400">{t("faceLab.findings.needBoth")}</p>}
                {sheet && (
                  <>
                    <div className="grid gap-3 lg:grid-cols-2">
                      <figure className="flex flex-col gap-1">
                        <img src={sheet.annotated} alt={t("faceLab.record.annotated")} className="w-full rounded-lg border border-gray-200 dark:border-gray-800" />
                        <figcaption className="break-all font-mono text-xs text-gray-500">{t("faceLab.record.annotated")} · SHA-256 {sheet.annotatedSha256}</figcaption>
                      </figure>
                      <figure className="flex flex-col gap-1">
                        <img src={sheet.clean} alt={t("faceLab.record.clean")} className="w-full rounded-lg border border-gray-200 dark:border-gray-800" />
                        <figcaption className="break-all font-mono text-xs text-gray-500">{t("faceLab.record.clean")} · SHA-256 {sheet.cleanSha256}</figcaption>
                      </figure>
                    </div>
                    <pre className="max-h-[40rem] overflow-auto whitespace-pre-wrap rounded-lg bg-gray-50 p-4 [font-family:inherit] text-base leading-relaxed text-gray-800 dark:bg-gray-900 dark:text-gray-200">
                      {sheet.record}
                    </pre>
                  </>
                )}
                <div className="text-sm text-gray-500 dark:text-gray-400">
                  <span className="font-medium">{t("faceLab.record.references")}</span>
                  <ul className="mt-1 list-disc pl-5">
                    {info.references.map((r) => <li key={r.code}>{r.title}, v. {r.version}, {r.sections}</li>)}
                  </ul>
                </div>
              </div>
            )}
          </ComponentCard>
        )}
      </div>
    </>
  );
}
