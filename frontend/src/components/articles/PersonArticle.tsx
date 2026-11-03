import { useCallback, useEffect, useRef, useState } from "react";
import { Link } from "react-router";
import { useTranslation } from "react-i18next";
import Button from "../ui/button/Button";
import {
  generatePersonArticleTemplate,
  getPersonArticle,
  getPersonBacklinks,
  getPersonLinkCounts,
  type Article,
  type LinkCounts,
  type ArticleTemplate,
  type Backlink,
} from "../../services/articles";
import type { PersonProfile } from "../../services/persons";
import ArticleView, { ArticleToc } from "./ArticleView";
import ArticleEditor from "./ArticleEditor";
import PersonInfobox from "./PersonInfobox";

/** "What links here": the other persons' articles that mention this one. */
function WhatLinksHere({ backlinks }: { backlinks: Backlink[] }) {
  const { t } = useTranslation();
  return (
    <div className="rounded-xl border border-gray-200 bg-white px-4 py-3 text-sm dark:border-gray-800 dark:bg-white/[0.03]">
      <p className="mb-1.5 font-semibold text-gray-800 dark:text-white/90">{t("articles.whatLinksHere")}</p>
      {backlinks.length === 0 ? (
        <p className="text-gray-500 dark:text-gray-400">{t("articles.noBacklinks")}</p>
      ) : (
        <ul className="space-y-1">
          {backlinks.map((b) => (
            <li key={`${b.subject.kind}:${b.subject.ref}`}>
              <Link
                to={`/persons/${encodeURIComponent(b.subject.ref)}`}
                className="text-brand-500 hover:underline dark:text-brand-400"
              >
                {b.subjectLabel}
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

/** A person's article tab: the article (or the editor), its contents, the generated infobox
 * and what links here. */
export default function PersonArticle({
  personId,
  profile,
  name,
  onChanged,
}: {
  personId: string;
  profile: PersonProfile;
  name: string;
  /** Called after a save, so the page can refresh what depends on the article (its history). */
  onChanged?: () => void;
}) {
  const { t, i18n } = useTranslation();
  const [article, setArticle] = useState<Article | null>(null);
  const [backlinks, setBacklinks] = useState<Backlink[]>([]);
  const [counts, setCounts] = useState<LinkCounts | null>(null);
  const [error, setError] = useState("");
  const [editing, setEditing] = useState(false);
  const [template, setTemplate] = useState<ArticleTemplate | undefined>(undefined);
  const [generating, setGenerating] = useState(false);
  const container = useRef<HTMLDivElement | null>(null);

  const load = useCallback(async () => {
    setError("");
    try {
      const [a, b] = await Promise.all([getPersonArticle(personId), getPersonBacklinks(personId)]);
      setArticle(a);
      setBacklinks(b);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("articles.loadError"));
    }
  }, [personId, t]);

  useEffect(() => {
    setEditing(false);
    setTemplate(undefined);
    load();
  }, [load]);

  // The counts by certainty are extra: the infobox shows the reviewed profile until they come.
  useEffect(() => {
    let cancelled = false;
    setCounts(null);
    getPersonLinkCounts(personId)
      .then((c) => !cancelled && setCounts(c))
      .catch(() => undefined);
    return () => {
      cancelled = true;
    };
  }, [personId]);

  // A first draft from the records, logged on the server; it opens in the editor, unsaved.
  const generate = async () => {
    setGenerating(true);
    setError("");
    try {
      setTemplate(await generatePersonArticleTemplate(personId, i18n.resolvedLanguage ?? "en"));
      setEditing(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("articles.template.error"));
    } finally {
      setGenerating(false);
    }
  };
  const stopEditing = () => {
    setEditing(false);
    setTemplate(undefined);
  };

  const revision = article?.revision ?? null;
  const locale = i18n.resolvedLanguage ?? "en";

  return (
    <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_18rem]">
      <div className="min-w-0 rounded-2xl border border-gray-200 bg-white px-5 py-5 sm:px-7 dark:border-gray-800 dark:bg-white/[0.03]">
        <div className="mb-1 flex flex-wrap items-start justify-between gap-3 border-b border-gray-200 pb-3 dark:border-gray-800">
          <div className="min-w-0">
            <h1 className="trackid-article-title text-2xl text-gray-900 dark:text-white/90">{name}</h1>
            <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {revision
                ? t("articles.lastEdited", {
                    date: new Date(revision.createdAt).toLocaleString(locale),
                    author: revision.author.name || revision.author.username,
                    count: revision.number,
                  })
                : t("articles.notWritten")}
              {revision?.templateId ? ` · ${t("articles.template.fromDraft")}` : ""}
            </p>
          </div>
          {!editing && article && revision && (
            <Button size="sm" type="button" onClick={() => setEditing(true)}>
              {t("articles.edit")}
            </Button>
          )}
        </div>
        <p className="mb-4 text-xs italic text-gray-500 dark:text-gray-400">{t("articles.disclaimer")}</p>

        {error && <p className="text-sm text-error-500">{error}</p>}
        {!article && !error && <p className="text-sm text-gray-500 dark:text-gray-400">{t("common.loading")}</p>}

        {article && editing && (
          <ArticleEditor
            key={template?.id ?? "edit"}
            personId={personId}
            article={article}
            template={template}
            onCancel={stopEditing}
            onSaved={(saved) => {
              setArticle(saved);
              stopEditing();
              onChanged?.();
            }}
          />
        )}

        {article && !editing && revision?.body && (
          <>
            <ArticleToc doc={revision.body} container={container} />
            <ArticleView doc={revision.body} mentions={article.mentions} container={container} />
          </>
        )}
        {article && !editing && revision?.redacted && (
          <p className="text-sm italic text-gray-500 dark:text-gray-400">{t("articles.redacted")}</p>
        )}
        {article && !editing && !revision && (
          <div className="rounded-xl border border-dashed border-gray-300 px-5 py-10 text-center text-sm text-gray-500 dark:border-gray-700 dark:text-gray-400">
            <p>{t("articles.empty")}</p>
            <p className="mx-auto mt-1 max-w-md text-xs">{t("articles.template.explain")}</p>
            <div className="mt-4 flex flex-wrap justify-center gap-3">
              <Button size="sm" type="button" onClick={generate} disabled={generating}>
                {generating ? t("articles.template.generating") : t("articles.template.generate")}
              </Button>
              <Button size="sm" variant="outline" type="button" onClick={() => setEditing(true)} disabled={generating}>
                {t("articles.template.blank")}
              </Button>
            </div>
          </div>
        )}
      </div>

      <div className="space-y-4">
        <PersonInfobox profile={profile} name={name} counts={counts} />
        <WhatLinksHere backlinks={backlinks} />
      </div>
    </div>
  );
}
