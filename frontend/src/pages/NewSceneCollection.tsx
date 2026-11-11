import { useEffect, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import PageMeta from "../components/common/PageMeta";
import PageBreadcrumb from "../components/common/PageBreadCrumb";
import ComponentCard from "../components/common/ComponentCard";
import Label from "../components/form/Label";
import FileDropzone from "../components/common/FileDropzone";
import { MATERIAL_FILES } from "../components/common/fileTypes";
import OccurrenceFields from "../components/intelligence/OccurrenceFields";
import { emptyOccurrence, occurrenceInput, occurrenceReady, type OccurrenceDraft } from "../components/intelligence/occurrenceDraft";
import { useCurrentUser } from "../context/CurrentUserContext";
import { getDistributionStatus, type DistributionExaminer } from "../services/analyses";
import { listOccurrenceTypes, searchOccurrences, type OccurrenceSummary, type OccurrenceType } from "../services/occurrences";
import { receiveCollectionMaterial, registerCollection, type ReportMode } from "../services/collections";
import { fetchTeam, getSceneNote, type SceneNote } from "../services/sceneNotes";
import type { TeamUser } from "../offline/fieldNotesStore";

const inputClass =
  "h-11 w-full rounded-lg border border-gray-300 bg-transparent px-4 text-sm text-gray-800 shadow-theme-xs placeholder:text-gray-400 focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90";
const buttonClass =
  "h-11 rounded-lg border border-gray-300 px-4 text-sm font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-50 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-white/[0.03]";

interface VestigeRow {
  description: string;
  sealNumber: string;
}

interface MaterialFile {
  file: File;
  /** The vestige row it was taken from, or -1. */
  vestige: number;
}

/** A datetime-local value for now, to the minute, in local time. */
function localNow(): string {
  const d = new Date();
  d.setSeconds(0, 0);
  return new Date(d.getTime() - d.getTimezoneOffset() * 60000).toISOString().slice(0, 16);
}

/** A datetime-local value for an ISO instant. */
function localOf(iso: string): string {
  const d = new Date(iso);
  d.setSeconds(0, 0);
  return new Date(d.getTime() - d.getTimezoneOffset() * 60000).toISOString().slice(0, 16);
}

function occurrenceLabel(o: OccurrenceSummary): string {
  return [o.trackidNumber, o.agency && o.number ? `${o.agency} ${o.number}` : o.number, o.inquiryNumber && `IPL ${o.inquiryNumber}`]
    .filter(Boolean)
    .join(" · ");
}

// Register a scene examined on call (docs/scene-collection-plan.md): the one way to open a case
// before a request. The occurrence (found, or registered here), who collected and when, the
// vestiges with their seals, and the files. The request that formalizes it joins the case later.
export default function NewSceneCollection() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { user, isCoordinator, isFingerprintExaminer } = useCurrentUser();
  const [types, setTypes] = useState<OccurrenceType[]>([]);
  const [creating, setCreating] = useState(false);
  const [query, setQuery] = useState("");
  const [matches, setMatches] = useState<OccurrenceSummary[] | null>(null);
  const [chosen, setChosen] = useState<OccurrenceSummary | null>(null);
  const [draft, setDraft] = useState<OccurrenceDraft>(emptyOccurrence);
  const [examiners, setExaminers] = useState<DistributionExaminer[]>([]);
  const [form, setForm] = useState({
    collectedById: 0,
    collectedFrom: localNow(),
    collectedTo: "",
    description: "",
    notes: "",
    nothingCollected: false,
    reportMode: "SEPARATE" as ReportMode,
    calledAt: "",
    otherOfficers: "",
  });
  const [team, setTeam] = useState<number[]>([]);
  const [teamList, setTeamList] = useState<TeamUser[]>([]);
  // The field note this collection is registered from (?note=), when it comes from one.
  const [params] = useSearchParams();
  const [note, setNote] = useState<SceneNote | null>(null);
  const [vestiges, setVestiges] = useState<VestigeRow[]>([{ description: "", sealNumber: "" }]);
  const [files, setFiles] = useState<MaterialFile[]>([]);
  const [saving, setSaving] = useState(false);
  const [progress, setProgress] = useState("");
  const [error, setError] = useState("");

  useEffect(() => {
    listOccurrenceTypes().then(setTypes).catch(() => setError(t("occurrenceForm.typesError")));
  }, [t]);

  // A coordinator registers the scene for whichever fingerprint examiner collected it.
  useEffect(() => {
    if (!isCoordinator) return;
    getDistributionStatus()
      .then((s) => setExaminers(s.examiners.filter((e) => e.examinesFingerprint)))
      .catch(() => setExaminers([]));
  }, [isCoordinator]);

  useEffect(() => {
    fetchTeam().then(setTeamList).catch(() => setTeamList([]));
  }, []);

  // A field note fills the form: its place and time for a new occurrence, and the collection's
  // own times, team and notes. Everything stays editable.
  useEffect(() => {
    const id = Number(params.get("note"));
    if (!id) return;
    getSceneNote(id)
      .then((n) => {
        if (n.collectionId) return;
        setNote(n);
        setCreating(true);
        setDraft((d) => ({
          ...d,
          occurredFrom: n.occurredAt ? localOf(n.occurredAt) : d.occurredFrom,
          place: n.lon !== undefined && n.lat !== undefined
            ? { lon: n.lon, lat: n.lat, precision: "EXACT", address: n.address, neighbourhood: n.neighbourhood, placeType: n.placeType,
                name: n.placeName, geocodeSource: n.geocodeSource, geocodeFlags: n.geocodeFlags }
            : d.place,
        }));
        setForm((f) => ({
          ...f,
          collectedFrom: n.examinedAt ? localOf(n.examinedAt) : f.collectedFrom,
          calledAt: n.calledAt ? localOf(n.calledAt) : "",
          otherOfficers: n.otherOfficers ?? "",
          notes: n.notes ?? "",
          collectedById: isCoordinator ? n.authorId : f.collectedById,
        }));
        setTeam(n.team.map((m) => m.id));
      })
      .catch((err) => setError(err instanceof Error ? err.message : String(err)));
  }, [params, isCoordinator]);

  const collector = form.collectedById || (isFingerprintExaminer ? user?.id ?? 0 : 0);

  async function lookUp() {
    setChosen(null);
    try {
      setMatches(await searchOccurrences(query.trim()));
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  const setVestige = (i: number, patch: Partial<VestigeRow>) => setVestiges(vestiges.map((v, j) => (j === i ? { ...v, ...patch } : v)));
  const filled = vestiges.filter((v) => v.description.trim() !== "");
  const occurrenceOK = creating ? occurrenceReady(draft) : chosen !== null;
  const ready = occurrenceOK && collector !== 0 && form.collectedFrom !== "" && (form.nothingCollected || filled.length > 0 || files.length > 0);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setSaving(true);
    try {
      // Rows left empty are dropped; files keep pointing at the rows that remain.
      const kept = vestiges.map((v, i) => ({ v, i })).filter(({ v }) => v.description.trim() !== "");
      const registered = await registerCollection({
        occurrenceId: creating ? undefined : chosen?.id,
        occurrence: creating ? occurrenceInput(draft, types) : undefined,
        description: form.description || undefined,
        collectedFrom: new Date(form.collectedFrom).toISOString(),
        collectedTo: form.collectedTo ? new Date(form.collectedTo).toISOString() : undefined,
        collectedById: collector,
        nothingCollected: form.nothingCollected,
        notes: form.notes || undefined,
        reportMode: form.reportMode,
        vestiges: form.nothingCollected ? [] : kept.map(({ v }) => ({ description: v.description, sealNumber: v.sealNumber || undefined })),
        calledAt: form.calledAt ? new Date(form.calledAt).toISOString() : undefined,
        otherOfficers: form.otherOfficers || undefined,
        team: team.filter((id) => id !== collector),
        sceneNoteId: note?.id,
      });
      toast.success(t("scene.registered", { caseId: registered.caseId }));

      // The collector works the case; a coordinator registering for someone else receives the files
      // through the coordinator's route.
      const asCoordinator = collector !== user?.id;
      if (!form.nothingCollected) {
        for (const [n, m] of files.entries()) {
          setProgress(t("requests.receiving", { done: n, total: files.length }));
          const row = kept.findIndex(({ i }) => i === m.vestige);
          try {
            await receiveCollectionMaterial(registered.caseId, registered.collectionId, m.file, row >= 0 ? registered.vestiges[row]?.id : undefined, asCoordinator);
          } catch (err) {
            toast.error(t("requests.materialError", { file: m.file.name, error: err instanceof Error ? err.message : String(err) }));
          }
        }
      }
      navigate(`/cases/${encodeURIComponent(registered.caseId)}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSaving(false);
      setProgress("");
    }
  }

  return (
    <>
      <PageMeta title={`${t("scene.title")} | TrackID`} description={t("scene.desc")} />
      <PageBreadcrumb pageTitle={t("scene.title")} />
      <form onSubmit={submit} className="space-y-6">
        {note && (
          <p className="rounded-lg border border-brand-200 bg-brand-50 px-4 py-3 text-sm text-brand-700 dark:border-brand-500/30 dark:bg-brand-500/10 dark:text-brand-300">
            {t("fieldNotes.fromNote", { when: new Date(note.writtenAt).toLocaleString() })}
            {note.audioCount > 0 && ` ${t("fieldNotes.audios", { count: note.audioCount })}.`}
          </p>
        )}
        <ComponentCard title={t("scene.occurrenceCard")} desc={t("scene.occurrenceCardDesc")}>
          <div className="flex flex-wrap items-end gap-3">
            <div className="min-w-64 flex-1">
              <Label htmlFor="scene-occurrence">{t("scene.findOccurrence")}</Label>
              <input id="scene-occurrence" className={inputClass} value={query} placeholder="005-01234/2026"
                onChange={(e) => setQuery(e.target.value)} />
            </div>
            <button type="button" className={buttonClass} disabled={query.trim().length < 2} onClick={() => { setCreating(false); void lookUp(); }}>
              {t("requests.lookUp")}
            </button>
            <button type="button" className={buttonClass} onClick={() => { setCreating(true); setChosen(null); }}>
              {t("requests.createOccurrence")}
            </button>
          </div>
          {!creating && chosen && (
            <p className="text-sm text-brand-600 dark:text-brand-400">
              {t("requests.chosen", { number: occurrenceLabel(chosen) })}{" "}
              <button type="button" className="underline" onClick={() => setChosen(null)}>{t("requests.change")}</button>
            </p>
          )}
          {!creating && !chosen && matches && matches.length === 0 && (
            <p className="text-sm text-gray-500 dark:text-gray-400">{t("requests.lookUpNone")}</p>
          )}
          {!creating && !chosen && matches && matches.length > 0 && (
            <ul className="space-y-1 text-sm">
              {matches.map((o) => (
                <li key={o.id} className="flex flex-wrap items-center gap-2 text-gray-700 dark:text-gray-300">
                  <span className="font-medium">{occurrenceLabel(o)}</span>
                  {o.occurredFrom && <span>{new Date(o.occurredFrom).toLocaleDateString()}</span>}
                  <span>{o.types.map((x) => t(`occurrenceType.${x}`)).join(", ")}</span>
                  {o.placeName && <span>{o.placeName}</span>}
                  <button type="button" className="text-brand-500 hover:underline" onClick={() => setChosen(o)}>
                    {t("requests.useThis")}
                  </button>
                </li>
              ))}
            </ul>
          )}
        </ComponentCard>

        {creating && <OccurrenceFields value={draft} onChange={setDraft} types={types} />}

        <ComponentCard title={t("scene.collectionCard")} desc={t("scene.collectionCardDesc")}>
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
            <div>
              <Label htmlFor="scene-from">{t("scene.collectedFrom")}</Label>
              <input id="scene-from" type="datetime-local" required className={inputClass} value={form.collectedFrom}
                onChange={(e) => setForm({ ...form, collectedFrom: e.target.value })} />
            </div>
            <div>
              <Label htmlFor="scene-to">{t("scene.collectedTo")}</Label>
              <input id="scene-to" type="datetime-local" className={inputClass} value={form.collectedTo}
                onChange={(e) => setForm({ ...form, collectedTo: e.target.value })} />
            </div>
            <div>
              <Label htmlFor="scene-collector">{t("scene.collector")}</Label>
              {isCoordinator ? (
                <select id="scene-collector" className={inputClass} value={collector || ""}
                  onChange={(e) => setForm({ ...form, collectedById: Number(e.target.value) })}>
                  <option value="" disabled>{t("scene.chooseCollector")}</option>
                  {examiners.map((x) => (
                    <option key={x.userId} value={x.userId}>{x.username}</option>
                  ))}
                </select>
              ) : (
                <input id="scene-collector" className={inputClass} value={user?.username ?? ""} disabled />
              )}
            </div>
            <div className="md:col-span-2">
              <Label htmlFor="scene-description">{t("scene.description")}</Label>
              <input id="scene-description" className={inputClass} value={form.description} placeholder={t("scene.descriptionPlaceholder")}
                onChange={(e) => setForm({ ...form, description: e.target.value })} />
            </div>
            <div>
              <Label htmlFor="scene-report-mode">{t("scene.reportMode")}</Label>
              <select id="scene-report-mode" className={inputClass} value={form.reportMode}
                onChange={(e) => setForm({ ...form, reportMode: e.target.value as ReportMode })}>
                <option value="SEPARATE">{t("reportMode.SEPARATE")}</option>
                <option value="TOGETHER">{t("reportMode.TOGETHER")}</option>
              </select>
            </div>
          </div>
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
            <div>
              <Label htmlFor="scene-called">{t("scene.calledAt")}</Label>
              <input id="scene-called" type="datetime-local" className={inputClass} value={form.calledAt}
                onChange={(e) => setForm({ ...form, calledAt: e.target.value })} />
            </div>
            <div className="md:col-span-1 xl:col-span-2">
              <Label htmlFor="scene-officers">{t("scene.otherOfficers")}</Label>
              <input id="scene-officers" className={inputClass} value={form.otherOfficers}
                onChange={(e) => setForm({ ...form, otherOfficers: e.target.value })} />
            </div>
          </div>
          {teamList.filter((m) => m.id !== collector).length > 0 && (
            <fieldset>
              <legend className="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-400">{t("scene.team")}</legend>
              <div className="flex flex-wrap gap-x-5 gap-y-2">
                {teamList.filter((m) => m.id !== collector).map((m) => (
                  <label key={m.id} className="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                    <input type="checkbox" checked={team.includes(m.id)}
                      onChange={(e) => setTeam(e.target.checked ? [...team, m.id] : team.filter((id) => id !== m.id))} />
                    {m.name || m.username}
                  </label>
                ))}
              </div>
            </fieldset>
          )}
          <div>
            <Label htmlFor="scene-notes">{t("scene.notes")}</Label>
            <textarea id="scene-notes" rows={3} value={form.notes} onChange={(e) => setForm({ ...form, notes: e.target.value })}
              className="w-full rounded-lg border border-gray-300 bg-transparent px-4 py-2.5 text-sm text-gray-800 dark:border-gray-700 dark:text-white/90" />
          </div>
          <label className="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
            <input type="checkbox" checked={form.nothingCollected} onChange={(e) => setForm({ ...form, nothingCollected: e.target.checked })} />
            {t("scene.nothingCollected")}
          </label>
        </ComponentCard>

        {!form.nothingCollected && (
          <ComponentCard title={t("scene.vestigesCard")} desc={t("scene.vestigesCardDesc")}>
            {vestiges.map((v, i) => (
              <div key={i} className="flex flex-wrap items-end gap-3">
                <span className="pb-3 text-sm font-medium text-gray-500 dark:text-gray-400">V{i + 1}</span>
                <div className="min-w-64 flex-1">
                  <Label htmlFor={`vestige-${i}`}>{t("collections.newVestige")}</Label>
                  <input id={`vestige-${i}`} className={inputClass} value={v.description} placeholder={t("collections.vestigePlaceholder")}
                    onChange={(e) => setVestige(i, { description: e.target.value })} />
                </div>
                <div className="w-48">
                  <Label htmlFor={`seal-${i}`}>{t("collections.sealNumber")}</Label>
                  <input id={`seal-${i}`} className={inputClass} value={v.sealNumber} onChange={(e) => setVestige(i, { sealNumber: e.target.value })} />
                </div>
                {vestiges.length > 1 && (
                  <button type="button" className="pb-3 text-sm text-error-500 hover:underline"
                    onClick={() => {
                      setVestiges(vestiges.filter((_, j) => j !== i));
                      setFiles(files.map((f) => ({ ...f, vestige: f.vestige === i ? -1 : f.vestige > i ? f.vestige - 1 : f.vestige })));
                    }}>
                    {t("common.remove")}
                  </button>
                )}
              </div>
            ))}
            <div>
              <button type="button" className={buttonClass} onClick={() => setVestiges([...vestiges, { description: "", sealNumber: "" }])}>
                {t("collections.addVestige")}
              </button>
            </div>
          </ComponentCard>
        )}

        {!form.nothingCollected && (
          <ComponentCard title={t("scene.filesCard")} desc={t("scene.filesCardDesc")}>
            <FileDropzone id="scene-files" multiple variant="compact" accept={MATERIAL_FILES}
              onFiles={(added) => setFiles([...files, ...added.map((file) => ({ file, vestige: -1 }))])} />
            {files.length > 0 && (
              <ul className="divide-y divide-gray-100 text-sm dark:divide-white/[0.05]">
                {files.map((m, f) => (
                  <li key={`${m.file.name}-${f}`} className="flex flex-wrap items-center justify-between gap-3 py-1.5 text-gray-700 dark:text-gray-300">
                    <span className="truncate">{m.file.name}</span>
                    <span className="flex items-center gap-3">
                      <select className="h-9 rounded-lg border border-gray-300 bg-transparent px-2 text-xs dark:border-gray-700 dark:bg-gray-900"
                        value={m.vestige} onChange={(e) => setFiles(files.map((x, j) => (j === f ? { ...x, vestige: Number(e.target.value) } : x)))}>
                        <option value={-1}>{t("origin.noVestige")}</option>
                        {vestiges.map((v, i) => v.description.trim() && (
                          <option key={i} value={i}>V{i + 1} · {v.description}</option>
                        ))}
                      </select>
                      <button type="button" className="text-error-500 hover:underline" onClick={() => setFiles(files.filter((_, j) => j !== f))}>
                        {t("common.remove")}
                      </button>
                    </span>
                  </li>
                ))}
              </ul>
            )}
          </ComponentCard>
        )}

        {error && <p className="text-sm text-error-500" role="alert">{error}</p>}
        {progress && <p className="text-sm text-gray-500 dark:text-gray-400">{progress}</p>}
        <div className="flex items-center justify-end gap-3">
          <Link to="/cases" className="h-11 rounded-lg border border-gray-300 px-5 py-2.5 text-sm font-medium text-gray-700 hover:bg-gray-50 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-white/[0.03]">
            {t("common.cancel")}
          </Link>
          <button type="submit" disabled={!ready || saving}
            className="h-11 rounded-lg bg-brand-500 px-5 text-sm font-medium text-white hover:bg-brand-600 disabled:opacity-50">
            {t("scene.save")}
          </button>
        </div>
      </form>
    </>
  );
}
