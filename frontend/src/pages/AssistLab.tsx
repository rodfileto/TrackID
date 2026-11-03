import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import PageMeta from "../components/common/PageMeta";
import PageBreadcrumb from "../components/common/PageBreadCrumb";
import ComponentCard from "../components/common/ComponentCard";
import Badge from "../components/ui/badge/Badge";
import {
  draftDocuments,
  extractDocument,
  getAssistInfo,
  type AssistDocument,
  type AssistDraft,
  type AssistInfo,
} from "../services/assist";

const buttonClass =
  "rounded-lg border border-gray-300 px-3 py-1.5 text-sm text-gray-700 hover:bg-gray-50 disabled:opacity-50 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-white/5";
const primaryClass =
  "rounded-lg bg-brand-500 px-3 py-1.5 text-sm font-medium text-white hover:bg-brand-600 disabled:opacity-50";

function short(sha256: string) {
  return sha256 ? sha256.slice(0, 12) : "";
}

function downloadText(name: string, text: string) {
  const url = URL.createObjectURL(new Blob([text], { type: "application/json" }));
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  a.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

export default function AssistLab() {
  const { t } = useTranslation();
  const [info, setInfo] = useState<AssistInfo | null>(null);
  const [documents, setDocuments] = useState<AssistDocument[]>([]);
  const [instructions, setInstructions] = useState("");
  const [draft, setDraft] = useState<AssistDraft | null>(null);
  const [drafting, setDrafting] = useState(false);
  const [draftError, setDraftError] = useState<string | null>(null);
  const [elapsed, setElapsed] = useState(0);

  // A local model can take a minute or more: show the seconds passing while it writes.
  useEffect(() => {
    if (!drafting) return;
    const start = Date.now();
    setElapsed(0);
    const id = setInterval(() => setElapsed(Math.floor((Date.now() - start) / 1000)), 1000);
    return () => clearInterval(id);
  }, [drafting]);

  useEffect(() => {
    getAssistInfo().then(setInfo).catch((err) => toast.error(err instanceof Error ? err.message : String(err)));
  }, []);

  async function addFiles(files: FileList | null) {
    if (!files) return;
    for (const file of Array.from(files)) {
      try {
        const doc = await extractDocument(file);
        setDocuments((docs) => [...docs, doc]);
      } catch (err) {
        toast.error(err instanceof Error ? err.message : String(err));
      }
    }
  }

  function setText(i: number, text: string) {
    setDocuments((docs) => docs.map((d, j) => (j === i ? { ...d, text } : d)));
  }

  function remove(i: number) {
    setDocuments((docs) => docs.filter((_, j) => j !== i));
  }

  async function runDraft() {
    setDrafting(true);
    setDraft(null);
    setDraftError(null);
    try {
      setDraft(await draftDocuments(documents, instructions));
    } catch (err) {
      // Kept on the page, not only in a toast: a draft that fails after minutes must not vanish.
      setDraftError(err instanceof Error ? err.message : String(err));
    } finally {
      setDrafting(false);
    }
  }

  const json = useMemo(() => (draft ? JSON.stringify(draft, null, 2) : ""), [draft]);
  const noText = documents.some((d) => !d.text.trim());
  const canDraft = documents.length > 0 || instructions.trim() !== "";

  return (
    <>
      <PageMeta title={`${t("assistLab.title")} | TrackID`} description={t("assistLab.desc")} />
      <PageBreadcrumb pageTitle={t("assistLab.title")} />
      <div className="flex flex-col gap-4">
        <ComponentCard title={t("assistLab.title")} desc={t("assistLab.desc")}>
          {info && !info.enabled && (
            <div className="mb-3">
              <Badge size="sm" color="warning">
                {t("assistLab.disabled")}
              </Badge>
            </div>
          )}
          {info?.enabled && info.model && (
            <p className="mb-3 text-sm text-gray-500 dark:text-gray-400">
              {t("assistLab.model", { model: info.model })}
            </p>
          )}

          <div className="mb-3 flex flex-col gap-1">
            <label htmlFor="assist-instructions" className="text-sm font-medium text-gray-700 dark:text-gray-300">
              {t("assistLab.instructions")}
            </label>
            <textarea
              id="assist-instructions"
              value={instructions}
              onChange={(e) => setInstructions(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && (e.ctrlKey || e.metaKey) && canDraft && !drafting) void runDraft();
              }}
              rows={4}
              className="w-full rounded-lg border border-gray-300 bg-transparent p-3 text-sm text-gray-800 focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-700 dark:text-white/90"
              placeholder={t("assistLab.instructionsPlaceholder")}
            />
            <span className="text-xs text-gray-500 dark:text-gray-400">{t("assistLab.instructionsHint")}</span>
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <label className={`${primaryClass} cursor-pointer`}>
              {documents.length ? t("assistLab.addMore") : t("assistLab.addFiles")}
              <input
                type="file"
                multiple
                accept="application/pdf,text/plain,text/*,image/*,.tif,.tiff,.bmp"
                className="hidden"
                onChange={(e) => {
                  void addFiles(e.target.files);
                  e.target.value = "";
                }}
              />
            </label>
            <button type="button" className={primaryClass} disabled={!canDraft || drafting} onClick={runDraft}>
              {drafting ? t("assistLab.drafting", { seconds: elapsed }) : t("assistLab.draft")}
            </button>
            {noText && <span className="text-sm text-gray-500 dark:text-gray-400">{t("assistLab.noTextHint")}</span>}
          </div>
        </ComponentCard>

        {documents.length > 0 && (
          <ComponentCard title={t("assistLab.documents")}>
            <div className="flex flex-col gap-4">
              {documents.map((doc, i) => (
                <div key={doc.sha256 + i} className="flex flex-col gap-2 rounded-lg border border-gray-200 p-3 dark:border-gray-800">
                  <div className="flex items-center gap-2 text-sm">
                    <span className="font-medium text-gray-800 dark:text-white/90">{doc.name}</span>
                    {doc.sha256 && <span className="font-mono text-xs text-gray-500 dark:text-gray-400">SHA-256 {short(doc.sha256)}…</span>}
                    <button type="button" className="ml-auto text-sm text-error-500" onClick={() => remove(i)}>
                      {t("assistLab.remove")}
                    </button>
                  </div>
                  <textarea
                    value={doc.text}
                    onChange={(e) => setText(i, e.target.value)}
                    spellCheck={false}
                    rows={Math.min(12, Math.max(3, doc.text.split("\n").length))}
                    className="w-full rounded-lg border border-gray-200 bg-gray-50 p-3 font-mono text-sm text-gray-800 dark:border-gray-800 dark:bg-gray-900 dark:text-gray-200"
                    placeholder={t("assistLab.noText")}
                  />
                </div>
              ))}
            </div>
          </ComponentCard>
        )}

        {draftError && (
          <div className="rounded-xl border border-error-500 bg-error-50 p-4 text-sm text-error-700 dark:border-error-500/40 dark:bg-error-500/10 dark:text-error-400">
            <div className="flex items-start gap-3">
              <div className="flex-1">
                <p className="font-medium">{t("assistLab.failed")}</p>
                <p className="mt-1 break-words font-mono text-xs">{draftError}</p>
              </div>
              <button type="button" className="text-sm underline" onClick={() => setDraftError(null)}>
                {t("assistLab.dismiss")}
              </button>
            </div>
          </div>
        )}

        {draft && (
          <ComponentCard title={t("assistLab.result")}>
            <div className="flex flex-col gap-3">
              {draft.usage && (
                <p className="font-mono text-xs text-gray-500 dark:text-gray-400">
                  {t("assistLab.usage", {
                    seconds: (draft.usage.durationMs / 1000).toFixed(1),
                    prompt: draft.usage.promptTokens,
                    output: draft.usage.completionTokens,
                    rate: draft.usage.durationMs > 0 ? ((draft.usage.completionTokens * 1000) / draft.usage.durationMs).toFixed(1) : "–",
                  })}
                  {draft.usage.reasoningTokens ? ` · ${t("assistLab.reasoningTokens", { count: draft.usage.reasoningTokens })}` : ""}
                </p>
              )}
              {draft.unsure && draft.unsure.length > 0 && (
                <div>
                  <span className="text-sm font-medium text-gray-700 dark:text-gray-300">{t("assistLab.unsure")}</span>
                  <div className="mt-1 flex flex-wrap gap-1">
                    {draft.unsure.map((u) => (
                      <Badge key={u} size="sm" color="warning">
                        {u}
                      </Badge>
                    ))}
                  </div>
                </div>
              )}
              {draft.reasoning && (
                <p className="text-sm text-gray-600 dark:text-gray-400">
                  <span className="font-medium">{t("assistLab.reasoning")}:</span> {draft.reasoning}
                </p>
              )}
              <div className="flex flex-wrap gap-2">
                <button type="button" className={buttonClass} onClick={() => navigator.clipboard?.writeText(json).then(() => toast.success(t("assistLab.copied")))}>
                  {t("assistLab.copy")}
                </button>
                <button type="button" className={buttonClass} onClick={() => downloadText("rascunho-registro.json", json)}>
                  {t("assistLab.download")}
                </button>
              </div>
              <pre className="max-h-[40rem] overflow-auto rounded-lg bg-gray-50 p-4 font-mono text-xs leading-relaxed text-gray-800 dark:bg-gray-900 dark:text-gray-200">{json}</pre>
            </div>
          </ComponentCard>
        )}
      </div>
    </>
  );
}
