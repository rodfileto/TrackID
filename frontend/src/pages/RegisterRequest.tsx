import { useEffect, useState } from "react";
import { Link, useNavigate } from "react-router";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import PageMeta from "../components/common/PageMeta";
import PageBreadcrumb from "../components/common/PageBreadCrumb";
import ComponentCard from "../components/common/ComponentCard";
import Label from "../components/form/Label";
import OccurrenceFields from "../components/intelligence/OccurrenceFields";
import {
  emptyOccurrence,
  occurrenceInput,
  occurrenceReady,
  type OccurrenceDraft,
} from "../components/intelligence/occurrenceDraft";
import { listOccurrenceTypes, searchOccurrences, type OccurrenceSummary, type OccurrenceType } from "../services/occurrences";
import { receiveMaterial, registerRequest, type RegisterRequestInput } from "../services/requests";

const inputClass =
  "h-11 w-full rounded-lg border border-gray-300 bg-transparent px-4 text-sm text-gray-800 shadow-theme-xs placeholder:text-gray-400 focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90";
const buttonClass =
  "h-11 rounded-lg border border-gray-300 px-4 text-sm font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-50 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-white/[0.03]";

/** How the request's occurrence is settled: one TrackID has, one registered here, or none. */
type OccurrenceMode = "lookup" | "create" | "none";

interface CaseRow {
  existing: boolean;
  caseId: string;
  modality: string;
  description: string;
  files: File[];
}

const newRow = (): CaseRow => ({ existing: false, caseId: "", modality: "FACIAL", description: "", files: [] });

function occurrenceLabel(o: OccurrenceSummary): string {
  return [o.trackidNumber, o.agency && o.number ? `${o.agency} ${o.number}` : o.number].filter(Boolean).join(" · ");
}

// Register a request as it arrives (docs/facial-reports-plan.md, "Request first"): the ofício,
// the occurrence it cites (found, or registered here), the cases it opens or joins, and the
// material sent in for each. The request and everything it opens are saved together; the files
// are received into their cases after.
export default function RegisterRequest() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [form, setForm] = useState({
    authority: "",
    documentReference: "",
    documentDate: "",
    receivedOn: new Date().toISOString().slice(0, 10),
    dueOn: "",
    citedReference: "",
    notes: "",
    questions: "",
  });
  const [types, setTypes] = useState<OccurrenceType[]>([]);
  const [mode, setMode] = useState<OccurrenceMode>("lookup");
  const [matches, setMatches] = useState<OccurrenceSummary[] | null>(null);
  const [chosen, setChosen] = useState<OccurrenceSummary | null>(null);
  const [draft, setDraft] = useState<OccurrenceDraft>(emptyOccurrence);
  const [caseType, setCaseType] = useState("CIVIL");
  const [rows, setRows] = useState<CaseRow[]>([newRow()]);
  const [saving, setSaving] = useState(false);
  const [progress, setProgress] = useState("");
  const [error, setError] = useState("");

  useEffect(() => {
    listOccurrenceTypes().then(setTypes).catch(() => setError(t("occurrenceForm.typesError")));
  }, [t]);

  const field = (key: keyof typeof form, type = "text", required = false) => (
    <div>
      <Label htmlFor={`request-${key}`}>{t(`requests.${key}`)}</Label>
      <input id={`request-${key}`} type={type} required={required} className={inputClass} value={form[key]}
        onChange={(e) => setForm({ ...form, [key]: e.target.value })} />
    </div>
  );

  async function lookUp() {
    setChosen(null);
    try {
      setMatches(await searchOccurrences(form.citedReference.trim()));
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  function startCreating() {
    setMode("create");
    setChosen(null);
    // The cited number is usually the police report's: carry it over.
    if (!draft.agencyNumber && form.citedReference) setDraft({ ...draft, agencyNumber: form.citedReference.trim() });
  }

  const setRow = (i: number, patch: Partial<CaseRow>) => setRows(rows.map((r, j) => (j === i ? { ...r, ...patch } : r)));

  const rowsReady = rows.every((r) => (r.existing ? r.caseId.trim() !== "" : r.description.trim() !== ""));
  const occurrenceOK = mode === "none" || (mode === "lookup" && chosen !== null) || (mode === "create" && occurrenceReady(draft));
  const ready = form.authority.trim() !== "" && form.documentReference.trim() !== "" && form.receivedOn !== "" && rowsReady && occurrenceOK;

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setSaving(true);
    try {
      const input: RegisterRequestInput = {
        authority: form.authority,
        documentReference: form.documentReference,
        documentDate: form.documentDate || undefined,
        receivedOn: form.receivedOn,
        dueOn: form.dueOn || undefined,
        citedReference: form.citedReference || undefined,
        notes: form.notes || undefined,
        questions: form.questions.split("\n").map((q) => q.trim()).filter(Boolean),
        occurrenceId: mode === "lookup" ? chosen?.id : undefined,
        occurrence: mode === "create" ? occurrenceInput(draft, types) : undefined,
        caseType: mode === "none" ? caseType : undefined,
        cases: rows.map((r) => (r.existing ? { caseId: r.caseId.trim() } : { modality: r.modality, description: r.description })),
      };
      const registered = await registerRequest(input);
      toast.success(t("requests.registered", { cases: registered.cases.map((c) => c.caseId).join(", ") }));

      // Each row's files go to the case it opened or joined (cases come back in the rows' order).
      const total = rows.reduce((n, r) => n + r.files.length, 0);
      let done = 0;
      for (const [i, row] of rows.entries()) {
        for (const file of row.files) {
          setProgress(t("requests.receiving", { done, total }));
          try {
            await receiveMaterial(registered.cases[i].caseId, registered.requestId, file);
          } catch (err) {
            toast.error(t("requests.materialError", { file: file.name, error: err instanceof Error ? err.message : String(err) }));
          }
          done++;
        }
      }
      navigate(`/cases/${encodeURIComponent(registered.cases[0].caseId)}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSaving(false);
      setProgress("");
    }
  }

  return (
    <>
      <PageMeta title={`${t("requests.new")} | TrackID`} description={t("requests.newDesc")} />
      <PageBreadcrumb pageTitle={t("requests.new")} />
      <form onSubmit={submit} className="space-y-6">
        <ComponentCard title={t("requests.requestCard")} desc={t("requests.requestCardDesc")}>
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
            {field("authority", "text", true)}
            {field("documentReference", "text", true)}
            {field("documentDate", "date")}
            {field("receivedOn", "date", true)}
            {field("dueOn", "date")}
            {field("notes")}
          </div>
          <div>
            <Label htmlFor="request-questions">{t("requests.questions")}</Label>
            <textarea id="request-questions" rows={4} value={form.questions} onChange={(e) => setForm({ ...form, questions: e.target.value })}
              className="w-full rounded-lg border border-gray-300 bg-transparent px-4 py-2.5 text-sm text-gray-800 shadow-theme-xs focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90" />
          </div>
        </ComponentCard>

        <ComponentCard title={t("requests.occurrenceCard")} desc={t("requests.occurrenceCardDesc")}>
          <div className="flex flex-wrap items-end gap-3">
            <div className="min-w-64 flex-1">
              <Label htmlFor="request-cited">{t("requests.citedReference")}</Label>
              <input id="request-cited" className={inputClass} value={form.citedReference} placeholder="005-01234/2026"
                onChange={(e) => setForm({ ...form, citedReference: e.target.value })} />
            </div>
            <button type="button" className={buttonClass} disabled={form.citedReference.trim().length < 2}
              onClick={() => { setMode("lookup"); void lookUp(); }}>
              {t("requests.lookUp")}
            </button>
            <button type="button" className={buttonClass} onClick={startCreating}>
              {t("requests.createOccurrence")}
            </button>
            <button type="button" className={buttonClass} onClick={() => { setMode("none"); setChosen(null); }}>
              {t("requests.noOccurrence")}
            </button>
          </div>

          {mode === "lookup" && chosen && (
            <p className="text-sm text-brand-600 dark:text-brand-400">
              {t("requests.chosen", { number: occurrenceLabel(chosen) })}{" "}
              <button type="button" className="underline" onClick={() => setChosen(null)}>{t("requests.change")}</button>
            </p>
          )}
          {mode === "lookup" && !chosen && matches && matches.length === 0 && (
            <p className="text-sm text-gray-500 dark:text-gray-400">{t("requests.lookUpNone")}</p>
          )}
          {mode === "lookup" && !chosen && matches && matches.length > 0 && (
            <ul className="space-y-1 text-sm">
              {matches.map((o) => (
                <li key={o.id} className="flex flex-wrap items-center gap-2 text-gray-700 dark:text-gray-300">
                  <span className="font-medium">{occurrenceLabel(o)}</span>
                  <span>{new Date(o.occurredFrom).toLocaleDateString()}</span>
                  <span>{o.types.map((x) => t(`occurrenceType.${x}`)).join(", ")}</span>
                  {o.placeName && <span>{o.placeName}</span>}
                  <button type="button" className="text-brand-500 hover:underline" onClick={() => setChosen(o)}>
                    {t("requests.useThis")}
                  </button>
                </li>
              ))}
            </ul>
          )}
          {mode === "none" && (
            <div className="max-w-xs">
              <Label htmlFor="request-nature">{t("requests.nature")}</Label>
              <select id="request-nature" className={inputClass} value={caseType} onChange={(e) => setCaseType(e.target.value)}>
                <option value="CIVIL">{t("caseType.civil")}</option>
                <option value="CRIMINAL">{t("caseType.criminal")}</option>
              </select>
            </div>
          )}
        </ComponentCard>

        {mode === "create" && <OccurrenceFields value={draft} onChange={setDraft} types={types} />}

        <ComponentCard title={t("requests.casesCard")} desc={t("requests.casesCardDesc")}>
          {rows.map((row, i) => (
            <div key={i} className="space-y-3 rounded-xl border border-gray-200 p-4 dark:border-gray-800">
              <div className="flex flex-wrap items-center gap-4 text-sm text-gray-700 dark:text-gray-300">
                <label className="flex items-center gap-2">
                  <input type="radio" checked={!row.existing} onChange={() => setRow(i, { existing: false })} />
                  {t("requests.newCase")}
                </label>
                <label className="flex items-center gap-2">
                  <input type="radio" checked={row.existing} onChange={() => setRow(i, { existing: true })} />
                  {t("requests.existingCase")}
                </label>
                {rows.length > 1 && (
                  <button type="button" className="ml-auto text-error-500 hover:underline" onClick={() => setRows(rows.filter((_, j) => j !== i))}>
                    {t("requests.removeCase")}
                  </button>
                )}
              </div>
              {row.existing ? (
                <div className="max-w-xs">
                  <Label htmlFor={`case-id-${i}`}>{t("requests.caseId")}</Label>
                  <input id={`case-id-${i}`} className={inputClass} value={row.caseId} onChange={(e) => setRow(i, { caseId: e.target.value })} />
                </div>
              ) : (
                <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
                  <div>
                    <Label htmlFor={`case-modality-${i}`}>{t("cases.columns.modality")}</Label>
                    <select id={`case-modality-${i}`} className={inputClass} value={row.modality} onChange={(e) => setRow(i, { modality: e.target.value })}>
                      <option value="FACIAL">{t("modality.facial")}</option>
                      <option value="FINGERPRINT">{t("modality.fingerprint")}</option>
                    </select>
                  </div>
                  <div className="md:col-span-2">
                    <Label htmlFor={`case-description-${i}`}>{t("cases.columns.description")}</Label>
                    <input id={`case-description-${i}`} className={inputClass} value={row.description}
                      placeholder={t("createCase.descriptionPlaceholder")} onChange={(e) => setRow(i, { description: e.target.value })} />
                  </div>
                </div>
              )}
              <div>
                <Label htmlFor={`case-files-${i}`}>{t("requests.caseMaterial")}</Label>
                <input id={`case-files-${i}`} type="file" multiple accept="image/*,application/pdf,video/*"
                  className="text-sm text-gray-700 dark:text-gray-300"
                  onChange={(e) => setRow(i, { files: Array.from(e.target.files ?? []) })} />
              </div>
            </div>
          ))}
          <div>
            <button type="button" className={buttonClass} onClick={() => setRows([...rows, newRow()])}>
              {t("requests.addCase")}
            </button>
          </div>
        </ComponentCard>

        {error && <p className="text-sm text-error-500" role="alert">{error}</p>}
        {progress && <p className="text-sm text-gray-500 dark:text-gray-400">{progress}</p>}
        <div className="flex items-center justify-end gap-3">
          <Link to="/requests" className="h-11 rounded-lg border border-gray-300 px-5 py-2.5 text-sm font-medium text-gray-700 hover:bg-gray-50 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-white/[0.03]">
            {t("common.cancel")}
          </Link>
          <button type="submit" disabled={!ready || saving}
            className="h-11 rounded-lg bg-brand-500 px-5 text-sm font-medium text-white hover:bg-brand-600 disabled:opacity-50">
            {t("requests.save")}
          </button>
        </div>
      </form>
    </>
  );
}
