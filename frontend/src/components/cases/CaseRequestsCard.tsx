import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import { Link } from "react-router";
import ComponentCard from "../common/ComponentCard";
import Badge from "../ui/badge/Badge";
import Button from "../ui/button/Button";
import { Modal } from "../ui/modal";
import { useCurrentUser } from "../../context/CurrentUserContext";
import type { Evidence } from "../../services/cases";
import { listCaseVideos } from "../../services/videos";
import {
  createCaseRequest,
  listCaseMaterial,
  receiveMaterial,
  listCaseRequests,
  setTraceRole,
  type CaseRequest,
  type CaseRequestInput,
  type MaterialTrace,
} from "../../services/requests";

const inputClass =
  "h-10 w-full rounded-lg border border-gray-300 bg-transparent px-3 text-sm text-gray-800 shadow-theme-xs focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90";
const labelClass = "mb-1 block text-sm font-medium text-gray-700 dark:text-gray-400";

function dateBR(iso?: string): string {
  if (!iso) return "";
  const [y, m, d] = iso.slice(0, 10).split("-");
  return `${d}/${m}/${y}`;
}

/** Coordinator: register a request -- who asks, by which document, when, what (the questions),
 * and which of the case's evidence files it sent in. */
function RegisterRequestModal({
  caseId,
  evidences,
  isOpen,
  onClose,
  onDone,
}: {
  caseId: string;
  evidences: Evidence[];
  isOpen: boolean;
  onClose: () => void;
  onDone: () => void;
}) {
  const { t } = useTranslation();
  const empty = { authority: "", documentReference: "", documentDate: "", receivedOn: new Date().toISOString().slice(0, 10), dueOn: "",
    citedReference: "", notes: "", questions: "" };
  const [form, setForm] = useState(empty);
  const [files, setFiles] = useState<Set<number>>(new Set());
  const [busy, setBusy] = useState(false);
  const [material, setMaterial] = useState<{ id: number; filename: string }[]>([]);

  useEffect(() => {
    if (!isOpen) return;
    setForm(empty);
    listCaseVideos(caseId)
      .catch(() => [])
      .then((videos) => {
        const all = [...evidences.map((e) => ({ id: e.id, filename: e.filename })), ...videos.map((v) => ({ id: v.fileId, filename: v.filename }))];
        setMaterial(all);
        setFiles(new Set(all.map((f) => f.id)));
      });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isOpen]);

  const field = (key: keyof typeof form, type = "text", required = false) => (
    <div>
      <label className={labelClass}>{t(`requests.${key}`)}</label>
      <input type={type} required={required} className={inputClass} value={form[key]} onChange={(e) => setForm({ ...form, [key]: e.target.value })} />
    </div>
  );

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    setBusy(true);
    try {
      const input: CaseRequestInput = {
        authority: form.authority, documentReference: form.documentReference, documentDate: form.documentDate || undefined,
        receivedOn: form.receivedOn, dueOn: form.dueOn || undefined, citedReference: form.citedReference || undefined,
        notes: form.notes || undefined, questions: form.questions.split("\n").map((q) => q.trim()).filter(Boolean), fileIds: [...files],
      };
      await createCaseRequest(caseId, input);
      toast.success(t("requests.saved"));
      onClose();
      onDone();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal isOpen={isOpen} onClose={onClose} className="max-w-2xl m-4">
      <form className="space-y-4 p-6" onSubmit={(e) => void submit(e)}>
        <h3 className="text-lg font-semibold text-gray-800 dark:text-white/90">{t("requests.registerTitle")}</h3>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          {field("authority", "text", true)}
          {field("documentReference", "text", true)}
          {field("documentDate", "date")}
          {field("receivedOn", "date", true)}
          {field("dueOn", "date")}
          {field("citedReference")}
        </div>
        <div>
          <label className={labelClass}>{t("requests.questions")}</label>
          <textarea rows={4} className={`${inputClass} h-auto py-2`} value={form.questions} onChange={(e) => setForm({ ...form, questions: e.target.value })} />
        </div>
        <div>
          <label className={labelClass}>{t("requests.notes")}</label>
          <input className={inputClass} value={form.notes} onChange={(e) => setForm({ ...form, notes: e.target.value })} />
        </div>
        {material.length > 0 && (
          <div>
            <label className={labelClass}>{t("requests.files")}</label>
            <ul className="max-h-40 space-y-1 overflow-y-auto text-sm">
              {material.map((e) => (
                <li key={e.id}>
                  <label className="flex items-center gap-2 text-gray-700 dark:text-gray-300">
                    <input
                      type="checkbox"
                      checked={files.has(e.id)}
                      onChange={() =>
                        setFiles((prev) => {
                          const next = new Set(prev);
                          if (next.has(e.id)) next.delete(e.id);
                          else next.add(e.id);
                          return next;
                        })
                      }
                    />
                    {e.filename}
                  </label>
                </li>
              ))}
            </ul>
          </div>
        )}
        <div className="flex justify-end gap-2">
          <Button size="sm" variant="outline" type="button" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button size="sm" type="submit" disabled={busy}>
            {t("requests.save")}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

/** A case's requests -- who asked, what, by when, and what they sent -- and its faces' roles:
 * questioned material, or a reference sent in for comparison. */
export default function CaseRequestsCard({
  caseId,
  evidences,
  refreshKey = 0,
  onChanged,
}: {
  caseId: string;
  evidences: Evidence[];
  refreshKey?: number;
  onChanged?: () => void;
}) {
  const { t } = useTranslation();
  const { isCoordinator, isExaminer } = useCurrentUser();
  const [requests, setRequests] = useState<CaseRequest[] | null>(null);
  const [material, setMaterial] = useState<MaterialTrace[]>([]);
  const [registering, setRegistering] = useState(false);
  const [receiving, setReceiving] = useState<number | null>(null);

  const load = useCallback(() => {
    listCaseRequests(caseId).then(setRequests).catch(() => setRequests([]));
    listCaseMaterial(caseId).then(setMaterial).catch(() => setMaterial([]));
  }, [caseId]);
  useEffect(load, [load, refreshKey]);

  /** Coordinator: receive more files a request sent in, into this case. */
  async function receive(requestId: number, files: FileList | null) {
    if (!files || files.length === 0) return;
    setReceiving(requestId);
    try {
      for (const file of Array.from(files)) {
        await receiveMaterial(caseId, requestId, file);
      }
      toast.success(t("requests.saved"));
      load();
      onChanged?.();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : String(err));
    } finally {
      setReceiving(null);
    }
  }

  async function toggleRole(m: MaterialTrace) {
    try {
      await setTraceRole(caseId, m.traceId, m.role === "QUESTIONED" ? "REFERENCE" : "QUESTIONED");
      toast.success(t("requests.roleSaved"));
      load();
      onChanged?.();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : String(err));
    }
  }

  return (
    <ComponentCard title={t("requests.title")} desc={t("requests.desc")}>
      {requests && requests.length === 0 && <p className="text-sm text-gray-500 dark:text-gray-400">{t("requests.none")}</p>}
      <ul className="space-y-4">
        {requests?.map((r) => (
          <li key={r.id} className="rounded-xl border border-gray-200 p-4 dark:border-gray-800">
            <div className="flex flex-wrap items-center gap-2">
              <span className="text-sm font-medium text-gray-800 dark:text-white/90">
                {t("requests.sequence", { sequence: r.sequence })} — {r.authority}, {r.documentReference}
              </span>
              {r.overdue && (
                <Badge size="sm" color="error">
                  {t("requests.overdue")}
                </Badge>
              )}
            </div>
            <p className="mt-1 text-theme-xs text-gray-500 dark:text-gray-400">
              {t("requests.receivedOn")} {dateBR(r.receivedOn)}
              {r.dueOn && ` · ${t("requests.dueOn")} ${dateBR(r.dueOn)}`}
              {" · "}
              {r.occurrence
                ? t("requests.occurrenceLinked", { number: r.occurrence.number ?? r.occurrence.occurrenceNumber })
                : r.citedReference
                  ? t("requests.occurrenceUnlinked", { reference: r.citedReference })
                  : ""}
              {r.registeredBy && ` · ${t("requests.registeredBy", { user: r.registeredBy })}`}
            </p>
            {r.cases.length > 1 && (
              <p className="mt-1 text-theme-xs text-gray-500 dark:text-gray-400">
                {t("requests.alsoIn")}{" "}
                {r.cases
                  .filter((c) => c.caseId !== caseId)
                  .map((c, i) => (
                    <span key={c.caseId}>
                      {i > 0 && ", "}
                      <Link to={`/cases/${encodeURIComponent(c.caseId)}`} className="text-brand-500 hover:underline">
                        {c.caseId}
                      </Link>{" "}
                      ({t(`modality.${c.modality.toLowerCase()}`)})
                    </span>
                  ))}
              </p>
            )}
            {r.questions.length > 0 && (
              <ol className="mt-2 list-decimal space-y-0.5 pl-5 text-sm text-gray-700 dark:text-gray-300">
                {r.questions.map((q, i) => (
                  <li key={i}>{q}</li>
                ))}
              </ol>
            )}
            {r.files.length > 0 && (
              <ul className="mt-2 space-y-0.5 text-theme-xs text-gray-500 dark:text-gray-400">
                {r.files.map((f) => (
                  <li key={f.id}>
                    {f.filename} · <span className="font-mono">{f.sha256.slice(0, 16)}…</span>
                  </li>
                ))}
              </ul>
            )}
            {isCoordinator && (
              <label className="mt-2 inline-flex cursor-pointer items-center gap-2 text-theme-xs text-brand-500 hover:underline">
                {receiving === r.id ? "…" : t("requests.receive")}
                <input type="file" multiple accept="image/*,application/pdf,video/*" className="hidden" disabled={receiving !== null}
                  onChange={(e) => void receive(r.id, e.target.files)} />
              </label>
            )}
          </li>
        ))}
      </ul>
      {isCoordinator && (
        <div className="mt-4">
          <Button size="sm" variant="outline" onClick={() => setRegistering(true)}>
            {t("requests.register")}
          </Button>
        </div>
      )}

      {material.length > 0 && (
        <div className="mt-6">
          <h4 className="mb-2 text-sm font-semibold text-gray-800 dark:text-white/90">{t("requests.material")}</h4>
          <ul className="space-y-1 text-sm">
            {material.map((m) => (
              <li key={m.traceId} className="flex flex-wrap items-center gap-2">
                <span className="text-gray-700 dark:text-gray-300">{m.label}</span>
                <Badge size="sm" color={m.role === "REFERENCE" ? "info" : "light"}>
                  {t(`requests.role.${m.role}`)}
                </Badge>
                {m.role === "QUESTIONED" && (
                  <Badge size="sm" color={m.suitability === "SUITABLE" ? "success" : m.suitability ? "warning" : "light"}>
                    {t(`analyses.suitability.${m.suitability ?? "none"}`)}
                  </Badge>
                )}
                {isExaminer && (
                  <button type="button" className="text-theme-xs text-brand-500 hover:underline" onClick={() => void toggleRole(m)}>
                    {m.role === "QUESTIONED" ? t("requests.makeReference") : t("requests.makeQuestioned")}
                  </button>
                )}
              </li>
            ))}
          </ul>
        </div>
      )}
      <RegisterRequestModal caseId={caseId} evidences={evidences} isOpen={registering} onClose={() => setRegistering(false)} onDone={load} />
    </ComponentCard>
  );
}
