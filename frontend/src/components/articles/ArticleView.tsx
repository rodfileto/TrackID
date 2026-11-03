import { useEffect, useMemo, useRef } from "react";
import { EditorContent, useEditor } from "@tiptap/react";
import { useTranslation } from "react-i18next";
import type { ArticleDoc, ResolvedMention } from "../../services/articles";
import { headingsOf } from "./headings";
import { articleExtensions } from "./extensions";
import { MentionContext, resolvedMap } from "./mentionContext";

/** The contents box: numbered headings, as Wikipedia shows them; a click scrolls to the
 * heading in the article rendered in container. */
export function ArticleToc({ doc, container }: { doc: ArticleDoc | undefined; container: React.RefObject<HTMLDivElement | null> }) {
  const { t } = useTranslation();
  const headings = useMemo(() => headingsOf(doc), [doc]);
  if (headings.length < 2) return null;
  let h2 = 0;
  let h3 = 0;
  const numbered = headings.map((h) => {
    if (h.level === 2) {
      h2 += 1;
      h3 = 0;
      return { ...h, number: `${h2}` };
    }
    h3 += 1;
    return { ...h, number: `${h2 || 0}.${h3}` };
  });
  const go = (index: number) => {
    const target = container.current?.querySelectorAll(".ProseMirror > h2, .ProseMirror > h3")[index];
    target?.scrollIntoView({ behavior: "smooth", block: "start" });
  };
  return (
    <nav className="mb-5 inline-block min-w-56 rounded-lg border border-gray-200 bg-gray-50 px-4 py-3 text-sm dark:border-gray-800 dark:bg-white/[0.02]">
      <p className="mb-1.5 font-semibold text-gray-800 dark:text-white/90">{t("articles.contents")}</p>
      <ol className="space-y-0.5">
        {numbered.map((h, i) => (
          <li key={i} className={h.level === 3 ? "pl-4" : ""}>
            <button
              type="button"
              onClick={() => go(i)}
              className="text-left text-brand-500 hover:underline dark:text-brand-400"
            >
              <span className="mr-1.5 text-gray-500 dark:text-gray-400">{h.number}</span>
              {h.text}
            </button>
          </li>
        ))}
      </ol>
    </nav>
  );
}

/** An article body, read-only, with its mentions as they read now. */
export default function ArticleView({
  doc,
  mentions,
  container,
}: {
  doc: ArticleDoc;
  mentions: ResolvedMention[];
  container?: React.RefObject<HTMLDivElement | null>;
}) {
  const own = useRef<HTMLDivElement | null>(null);
  const ref = container ?? own;
  const editor = useEditor({ extensions: articleExtensions(false), content: doc, editable: false }, []);
  useEffect(() => {
    if (editor && !editor.isDestroyed) editor.commands.setContent(doc, { emitUpdate: false });
  }, [editor, doc]);
  const value = useMemo(() => ({ resolved: resolvedMap(mentions), editing: false }), [mentions]);
  return (
    <MentionContext.Provider value={value}>
      <div ref={ref} className="trackid-article">
        <EditorContent editor={editor} />
      </div>
    </MentionContext.Provider>
  );
}
