import { useEffect, useMemo, useState } from "react";
import { EditorContent, useEditor, type Editor } from "@tiptap/react";
import { useTranslation } from "react-i18next";
import Button from "../ui/button/Button";
import {
  savePersonArticle,
  type Article,
  type ArticleDoc,
  type ArticleTemplate,
  type MentionKind,
} from "../../services/articles";
import { articleExtensions, isHttp } from "./extensions";
import { MentionContext, resolvedMap } from "./mentionContext";
import ArticleView from "./ArticleView";

const EMPTY_DOC: ArticleDoc = { type: "doc", content: [{ type: "paragraph" }] };

function ToolbarButton({
  label,
  active,
  onClick,
  children,
}: {
  label: string;
  active?: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      title={label}
      aria-label={label}
      aria-pressed={active}
      onMouseDown={(e) => e.preventDefault()}
      onClick={onClick}
      className={`min-w-8 rounded-md px-2 py-1 text-sm font-medium ${
        active
          ? "bg-brand-50 text-brand-700 dark:bg-brand-500/15 dark:text-brand-300"
          : "text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-white/10"
      }`}
    >
      {children}
    </button>
  );
}

function Toolbar({ editor, onLinkError }: { editor: Editor; onLinkError: (message: string) => void }) {
  const { t } = useTranslation();
  const chain = () => editor.chain().focus();
  const setLink = () => {
    const previous = (editor.getAttributes("link").href as string | undefined) ?? "";
    const url = window.prompt(t("articles.editor.linkPrompt"), previous || "https://");
    if (url === null) return;
    if (url.trim() === "") {
      chain().extendMarkRange("link").unsetLink().run();
      return;
    }
    if (!isHttp(url.trim())) {
      onLinkError(t("articles.editor.linkInvalid"));
      return;
    }
    chain().extendMarkRange("link").setLink({ href: url.trim() }).run();
  };
  return (
    <div className="flex flex-wrap items-center gap-1 border-b border-gray-200 px-2 py-1.5 dark:border-gray-800">
      <ToolbarButton label={t("articles.editor.heading2")} active={editor.isActive("heading", { level: 2 })} onClick={() => chain().toggleHeading({ level: 2 }).run()}>
        H2
      </ToolbarButton>
      <ToolbarButton label={t("articles.editor.heading3")} active={editor.isActive("heading", { level: 3 })} onClick={() => chain().toggleHeading({ level: 3 }).run()}>
        H3
      </ToolbarButton>
      <span className="mx-1 h-5 w-px bg-gray-200 dark:bg-gray-700" />
      <ToolbarButton label={t("articles.editor.bold")} active={editor.isActive("bold")} onClick={() => chain().toggleBold().run()}>
        <span className="font-bold">B</span>
      </ToolbarButton>
      <ToolbarButton label={t("articles.editor.italic")} active={editor.isActive("italic")} onClick={() => chain().toggleItalic().run()}>
        <span className="italic">I</span>
      </ToolbarButton>
      <ToolbarButton label={t("articles.editor.link")} active={editor.isActive("link")} onClick={setLink}>
        🔗
      </ToolbarButton>
      <span className="mx-1 h-5 w-px bg-gray-200 dark:bg-gray-700" />
      <ToolbarButton label={t("articles.editor.bulletList")} active={editor.isActive("bulletList")} onClick={() => chain().toggleBulletList().run()}>
        •≡
      </ToolbarButton>
      <ToolbarButton label={t("articles.editor.orderedList")} active={editor.isActive("orderedList")} onClick={() => chain().toggleOrderedList().run()}>
        1≡
      </ToolbarButton>
      <ToolbarButton label={t("articles.editor.quote")} active={editor.isActive("blockquote")} onClick={() => chain().toggleBlockquote().run()}>
        ❝
      </ToolbarButton>
      <span className="mx-1 h-5 w-px bg-gray-200 dark:bg-gray-700" />
      <ToolbarButton label={t("articles.editor.mention")} onClick={() => chain().insertContent("@").run()}>
        @
      </ToolbarButton>
      <span className="mx-1 h-5 w-px bg-gray-200 dark:bg-gray-700" />
      <ToolbarButton label={t("articles.editor.undo")} onClick={() => chain().undo().run()}>
        ↶
      </ToolbarButton>
      <ToolbarButton label={t("articles.editor.redo")} onClick={() => chain().redo().run()}>
        ↷
      </ToolbarButton>
    </div>
  );
}

/** Editing a person's article: the text with its toolbar and @ menu, an edit summary, and the
 * save. A save based on a revision someone else has since replaced comes back as a conflict:
 * the newer version is shown beside the edit for the analyst to merge by hand, then saved on
 * top of it. */
export default function ArticleEditor({
  personId,
  article,
  template,
  onSaved,
  onCancel,
}: {
  personId: string;
  article: Article;
  /** A draft generated from the records to start from, instead of the current text. */
  template?: ArticleTemplate;
  onSaved: (article: Article) => void;
  onCancel: () => void;
}) {
  const { t, i18n } = useTranslation();
  const [summary, setSummary] = useState(template ? t("articles.template.summary") : "");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [missing, setMissing] = useState<{ kind: MentionKind; ref: string }[]>([]);
  const [baseRevisionId, setBaseRevisionId] = useState<number | null>(article.revision?.id ?? null);
  const [conflict, setConflict] = useState<Article | null>(null);
  const [dirty, setDirty] = useState(false);

  const editor = useEditor(
    {
      extensions: articleExtensions(true),
      content: template?.body ?? article.revision?.body ?? EMPTY_DOC,
      shouldRerenderOnTransaction: true,
      autofocus: "end",
      onUpdate: () => setDirty(true),
    },
    [],
  );

  // Leaving with unsaved text asks first.
  useEffect(() => {
    if (!dirty) return;
    const warn = (event: BeforeUnloadEvent) => event.preventDefault();
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [dirty]);

  const mentionValue = useMemo(() => ({ resolved: resolvedMap(article.mentions), editing: true }), [article.mentions]);

  const save = async () => {
    if (!editor) return;
    setSaving(true);
    setError("");
    setMissing([]);
    try {
      const result = await savePersonArticle(personId, baseRevisionId, editor.getJSON() as ArticleDoc, summary, template?.id);
      if (result.status === "saved") {
        setDirty(false);
        onSaved(result.article);
      } else if (result.status === "conflict") {
        setConflict(result.article);
      } else {
        setError(result.error || t("articles.editor.refused"));
        setMissing(result.missing);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : t("articles.editor.saveError"));
    } finally {
      setSaving(false);
    }
  };

  const cancel = () => {
    if (dirty && !window.confirm(t("articles.editor.discardConfirm"))) return;
    onCancel();
  };

  const newer = conflict?.revision;
  return (
    <div className="space-y-4">
      {template && (
        <div className="rounded-xl border border-brand-200 bg-brand-25 p-4 text-sm text-gray-700 dark:border-brand-500/30 dark:bg-brand-500/10 dark:text-gray-300">
          <p className="font-medium text-gray-800 dark:text-white/90">
            {t("articles.template.banner", {
              date: new Date(template.generatedAt).toLocaleString(i18n.resolvedLanguage ?? "en"),
            })}
          </p>
          <p className="mt-1">{t("articles.template.help")}</p>
        </div>
      )}
      {conflict && newer && (
        <div className="rounded-xl border border-warning-300 bg-warning-50 p-4 text-sm text-warning-800 dark:border-warning-500/40 dark:bg-warning-500/10 dark:text-warning-300">
          <p className="font-medium">
            {t("articles.editor.conflictTitle", {
              number: newer.number,
              author: newer.author.name || newer.author.username,
              date: new Date(newer.createdAt).toLocaleString(i18n.resolvedLanguage ?? "en"),
            })}
          </p>
          <p className="mt-1">{t("articles.editor.conflictHelp")}</p>
          <div className="mt-3">
            <Button
              size="sm"
              variant="outline"
              type="button"
              onClick={() => {
                setBaseRevisionId(newer.id);
                setConflict(null);
              }}
            >
              {t("articles.editor.conflictMerged", { number: newer.number })}
            </Button>
          </div>
        </div>
      )}

      <div className={conflict?.revision?.body ? "grid gap-4 xl:grid-cols-2" : ""}>
        <div className="overflow-hidden rounded-xl border border-gray-200 bg-white dark:border-gray-800 dark:bg-white/[0.03]">
          {editor && <Toolbar editor={editor} onLinkError={setError} />}
          <MentionContext.Provider value={mentionValue}>
            <div className="trackid-article trackid-article-editing px-5 py-4">
              <EditorContent editor={editor} />
            </div>
          </MentionContext.Provider>
        </div>
        {conflict?.revision?.body && (
          <div className="rounded-xl border border-warning-300 bg-white px-5 py-4 dark:border-warning-500/40 dark:bg-white/[0.03]">
            <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-warning-700 dark:text-warning-400">
              {t("articles.editor.newerVersion", { number: conflict.revision.number })}
            </p>
            <ArticleView doc={conflict.revision.body} mentions={conflict.mentions} />
          </div>
        )}
      </div>

      <p className="text-xs text-gray-500 dark:text-gray-400">{t("articles.editor.mentionHelp")}</p>

      {error && (
        <div className="rounded-lg bg-error-50 px-4 py-3 text-sm text-error-700 dark:bg-error-500/10 dark:text-error-400">
          <p>{error}</p>
          {missing.length > 0 && (
            <ul className="mt-1 list-disc pl-5">
              {missing.map((m) => (
                <li key={`${m.kind}:${m.ref}`}>
                  {t(`articles.kind.${m.kind}`)} {m.ref}
                </li>
              ))}
            </ul>
          )}
        </div>
      )}

      <div className="flex flex-wrap items-end gap-3">
        <label className="min-w-64 flex-1 text-sm">
          <span className="mb-1 block text-gray-700 dark:text-gray-300">{t("articles.editor.summary")}</span>
          <input
            type="text"
            value={summary}
            maxLength={300}
            onChange={(e) => setSummary(e.target.value)}
            placeholder={t("articles.editor.summaryPlaceholder")}
            className="h-10 w-full rounded-lg border border-gray-300 bg-transparent px-3 text-sm text-gray-800 focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-700 dark:text-white/90"
          />
        </label>
        <Button type="button" variant="outline" onClick={cancel} disabled={saving}>
          {t("common.cancel")}
        </Button>
        <Button type="button" onClick={save} disabled={saving || !editor || conflict !== null}>
          {saving ? t("articles.editor.saving") : t("articles.editor.save")}
        </Button>
      </div>
    </div>
  );
}
