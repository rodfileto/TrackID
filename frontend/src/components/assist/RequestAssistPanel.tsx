import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import ComponentCard from "../common/ComponentCard";
import FileDropzone from "../common/FileDropzone";
import Label from "../form/Label";
import { DOCUMENT_FILES } from "../common/fileTypes";
import {
  draftDocuments,
  extractDocument,
  getAssistInfo,
  type AssistDocument,
  type AssistDraft,
  type AssistInfo,
  type AssistKind,
} from "../../services/assist";

/** A file the operator added, read: what it says and whether it is a document or an evidence. */
export interface ReadFile {
  file: File;
  doc: AssistDocument;
}

interface Props {
  /** Called with the draft and the files as the operator left them, to fill the form. */
  onDraft: (draft: AssistDraft, files: ReadFile[]) => void;
}

// Fill a request from what it sent: the ePol task or ofício, as text, PDF or a picture, and the
// evidence images. Each file is read on the server -- images and scanned pages through the OCR
// model -- and marked document or evidence, which the operator can change. Only documents' text
// drafts the request; evidences go to the case as material. The draft fills the form and nothing
// more: the operator checks it and registers.
export default function RequestAssistPanel({ onDraft }: Props) {
  const { t } = useTranslation();
  const [info, setInfo] = useState<AssistInfo | null>(null);
  const [files, setFiles] = useState<ReadFile[]>([]);
  const [reading, setReading] = useState("");
  const [instructions, setInstructions] = useState("");
  const [drafting, setDrafting] = useState(false);
  const [result, setResult] = useState<AssistDraft | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    getAssistInfo().then(setInfo).catch(() => setInfo(null));
  }, []);

  if (!info?.enabled) return null;

  async function add(added: File[]) {
    setError("");
    for (const [i, file] of added.entries()) {
      setReading(t("assistRequest.reading", { file: file.name, done: i + 1, total: added.length }));
      try {
        const doc = await extractDocument(file);
        setFiles((current) => [...current, { file, doc: { ...doc, kind: doc.kind ?? "document" } }]);
      } catch (err) {
        setError(err instanceof Error ? err.message : String(err));
      }
    }
    setReading("");
  }

  const setKind = (i: number, kind: AssistKind) =>
    setFiles((current) => current.map((f, j) => (j === i ? { ...f, doc: { ...f.doc, kind } } : f)));

  async function draft() {
    setError("");
    setDrafting(true);
    try {
      const d = await draftDocuments(files.map((f) => f.doc), instructions);
      setResult(d);
      onDraft(d, files);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setDrafting(false);
    }
  }

  const unsure = (result?.unsure ?? []).map((field) => t(`requests.${field}`, { defaultValue: field }));

  return (
    <ComponentCard title={t("assistRequest.title")} desc={t(info.ocr ? "assistRequest.desc" : "assistRequest.descNoOcr")}>
      <FileDropzone multiple variant="compact" accept={DOCUMENT_FILES} disabled={reading !== "" || drafting} onFiles={(f) => void add(f)} />
      {reading && <p className="text-sm text-gray-500 dark:text-gray-400">{reading}</p>}
      {files.length > 0 && (
        <ul className="divide-y divide-gray-100 text-sm dark:divide-white/[0.05]">
          {files.map((f, i) => (
            <li key={`${f.doc.sha256}-${i}`} className="flex flex-wrap items-center gap-3 py-2 text-gray-700 dark:text-gray-300">
              <span className="min-w-0 flex-1 truncate">{f.file.name}</span>
              <span className="text-theme-xs text-gray-500 dark:text-gray-400">
                {f.doc.text ? t("assistRequest.chars", { count: f.doc.text.length }) : t("assistRequest.noText")}
                {f.doc.ocr && ` · ${t("assistRequest.ocr")}`}
              </span>
              <select
                className="h-9 rounded-lg border border-gray-300 bg-transparent px-2 text-sm dark:border-gray-700 dark:bg-gray-900"
                value={f.doc.kind}
                aria-label={t("assistRequest.kind", { file: f.file.name })}
                onChange={(e) => setKind(i, e.target.value as AssistKind)}
              >
                <option value="document">{t("assistRequest.kindDocument")}</option>
                <option value="evidence">{t("assistRequest.kindEvidence")}</option>
              </select>
              <button type="button" className="text-error-500 hover:underline" onClick={() => setFiles(files.filter((_, j) => j !== i))}>
                {t("common.remove")}
              </button>
            </li>
          ))}
        </ul>
      )}
      <div>
        <Label htmlFor="assist-instructions">{t("assistRequest.instructions")}</Label>
        <textarea
          id="assist-instructions"
          rows={2}
          value={instructions}
          placeholder={t("assistRequest.instructionsPlaceholder")}
          onChange={(e) => setInstructions(e.target.value)}
          className="w-full rounded-lg border border-gray-300 bg-transparent px-4 py-2.5 text-sm text-gray-800 dark:border-gray-700 dark:text-white/90"
        />
      </div>
      <div className="flex items-center gap-3">
        <button
          type="button"
          onClick={() => void draft()}
          disabled={drafting || reading !== "" || (files.length === 0 && instructions.trim() === "")}
          className="h-11 rounded-lg bg-brand-500 px-5 text-sm font-medium text-white hover:bg-brand-600 disabled:opacity-50"
        >
          {drafting ? t("assistRequest.drafting") : t("assistRequest.fill")}
        </button>
        {drafting && <span className="text-theme-xs text-gray-500 dark:text-gray-400">{t("assistRequest.draftingHint")}</span>}
      </div>
      {error && <p className="text-sm text-error-500" role="alert">{error}</p>}
      {result && (
        <div className="rounded-lg border border-brand-200 bg-brand-50 px-4 py-3 text-sm text-brand-800 dark:border-brand-500/30 dark:bg-brand-500/10 dark:text-brand-300" role="status">
          <p className="font-medium">{t("assistRequest.filled")}</p>
          {result.reasoning && <p className="mt-1">{result.reasoning}</p>}
          {unsure.length > 0 && <p className="mt-1">{t("assistRequest.unsure", { fields: unsure.join(", ") })}</p>}
        </div>
      )}
    </ComponentCard>
  );
}
