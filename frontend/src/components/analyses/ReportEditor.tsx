import { useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import Button from "../ui/button/Button";
import { getReportDraft, saveReportDraft } from "../../services/analyses";
import { useCurrentUser } from "../../context/CurrentUserContext";

interface Props {
  caseId: string;
  analysisId: number;
  title: string;
  onClose: (saved: boolean) => void;
}

/** The report's draft in WordCraft's web build, in a frame on this origin. WordCraft's host bridge
 * (?host=parent) asks for the document when it is ready (wordcraft:ready), and hands every save
 * back (wordcraft:save) instead of downloading it; the draft is protected, so only its editable
 * sections can change, and the server checks that again on every save. */
export default function ReportEditor({ caseId, analysisId, title, onClose }: Props) {
  const { t } = useTranslation();
  const { user } = useCurrentUser();
  const frame = useRef<HTMLIFrameElement>(null);
  const [version, setVersion] = useState(0);
  const [dirty, setDirty] = useState(false);
  const [ready, setReady] = useState(false);
  const [failed, setFailed] = useState("");
  const saved = useRef(false);

  useEffect(() => {
    let cancelled = false;
    async function onMessage(event: MessageEvent) {
      if (event.origin !== window.location.origin || event.source !== frame.current?.contentWindow) return;
      const data = event.data as { type?: string; bytes?: ArrayBuffer; dirty?: boolean; error?: string; saveAs?: boolean; name?: string };
      switch (data.type) {
        case "wordcraft:ready": {
          setReady(true);
          try {
            const draft = await getReportDraft(caseId, analysisId);
            if (cancelled) return;
            setVersion(draft.version);
            if (draft.stale) toast.warning(t("reports.stale"));
            frame.current?.contentWindow?.postMessage(
              { type: "wordcraft:open", name: `${caseId}-v${draft.version}.docx`, bytes: draft.bytes },
              window.location.origin,
              [draft.bytes],
            );
          } catch (err) {
            setFailed(err instanceof Error ? err.message : String(err));
          }
          break;
        }
        case "wordcraft:save": {
          // Exports (a PDF, a picture) come through here too; only the document itself is a draft.
          if (!data.bytes || !(data.name ?? "").toLowerCase().endsWith(".docx")) return;
          try {
            const v = await saveReportDraft(caseId, analysisId, data.bytes);
            saved.current = true;
            setVersion(v.version);
            toast.success(t("reports.saved", { version: v.version }));
          } catch (err) {
            toast.error(err instanceof Error ? err.message : String(err));
          }
          break;
        }
        case "wordcraft:dirty":
          setDirty(!!data.dirty);
          break;
        case "wordcraft:failed":
          setFailed(data.error ?? t("reports.editorUnavailable"));
          break;
      }
    }
    window.addEventListener("message", onMessage);
    return () => {
      cancelled = true;
      window.removeEventListener("message", onMessage);
    };
  }, [caseId, analysisId, t]);

  const author = encodeURIComponent(user ? `${user.first_name} ${user.last_name}`.trim() || user.username : "");
  return (
    <div className="fixed inset-0 z-99999 flex flex-col bg-white dark:bg-gray-900">
      <div className="flex flex-wrap items-center gap-3 border-b border-gray-200 px-4 py-2 dark:border-gray-800">
        <span className="text-sm font-medium text-gray-800 dark:text-white/90">
          {t("reports.editorTitle", { title, version })}
        </span>
        {dirty && <span className="text-xs text-warning-600">{t("reports.unsaved")}</span>}
        <div className="ml-auto">
          <Button
            size="sm"
            variant="outline"
            onClick={() => {
              if (dirty && !window.confirm(t("reports.unsaved") + " — " + t("reports.close") + "?")) return;
              onClose(saved.current);
            }}
          >
            {t("reports.close")}
          </Button>
        </div>
      </div>
      {failed ? (
        <p className="p-6 text-sm text-error-500">{failed}</p>
      ) : (
        <>
          {!ready && <p className="p-6 text-sm text-gray-500">{t("reports.editorLoading")}</p>}
          <iframe
            ref={frame}
            title={title}
            src={`/wordcraft/?host=parent&author=${author}`}
            className={`w-full flex-1 border-0 ${ready ? "" : "h-0"}`}
          />
        </>
      )}
    </div>
  );
}
