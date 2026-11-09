import { apiBaseUrl, apiFetch } from "./api";
import { getToken } from "./auth";

/** Person articles (docs/person-article-plan.md): what intelligence analysts write about a
 * person, with @ mentions. Mirrors the Go package articles. */

export type MentionKind =
  | "CASE"
  | "OCCURRENCE"
  | "INQUIRY"
  | "PERSON"
  | "CLUSTER"
  | "REQUEST"
  | "REPORT";

/** Every mention kind, in the order the @ menu groups them. */
export const MENTION_KINDS: MentionKind[] = [
  "CASE",
  "OCCURRENCE",
  "INQUIRY",
  "PERSON",
  "CLUSTER",
  "REQUEST",
  "REPORT",
];

/** A mention target as the @ menu and a chip show it (articles.Target). */
export interface MentionTarget {
  kind: MentionKind;
  ref: string;
  personNumber?: number;
  label: string;
  caseIds?: string[];
  description?: string;
  caseType?: string;
  modality?: string;
  status?: string;
  basis?: string;
  reportType?: string;
  date?: string;
  count?: number;
}

/** One mention as it reads now (articles.Resolved). */
export interface ResolvedMention {
  kind: MentionKind;
  ref: string;
  found: boolean;
  mergedInto?: string;
  target?: MentionTarget;
}

/** A ProseMirror document as TipTap produces it. */
export interface ArticleDoc {
  type: "doc";
  content?: ArticleNode[];
}

export interface ArticleNode {
  type: string;
  attrs?: Record<string, unknown>;
  content?: ArticleNode[];
  text?: string;
  marks?: { type: string; attrs?: Record<string, unknown> }[];
}

export interface ArticleAuthor {
  id: number;
  username: string;
  name: string;
}

export interface ArticleRevision {
  id: number;
  number: number;
  parentId?: number;
  sha256?: string;
  chars: number;
  sizeDelta: number;
  editSummary?: string;
  author: ArticleAuthor;
  createdAt: string;
  redacted?: boolean;
  /** The draft generated from the records this revision was saved from. */
  templateId?: number;
  body?: ArticleDoc;
  text?: string;
}

export interface Article {
  subject: { kind: string; ref: string };
  revision: ArticleRevision | null;
  mentions: ResolvedMention[];
  createdAt?: string;
  updatedAt?: string;
}

export interface Backlink {
  subject: { kind: string; ref: string };
  subjectLabel: string;
  subjectNumber?: number;
  mentions: { kind: MentionKind; ref: string; count: number }[];
  updatedAt: string;
}

function headers(): Record<string, string> {
  const token = getToken();
  return {
    "Content-Type": "application/json",
    ...(token ? { Authorization: `Bearer ${token}` } : {}),
  };
}

async function getJSON<T>(path: string, signal?: AbortSignal): Promise<T> {
  const response = await apiFetch(`${apiBaseUrl}${path}`, { headers: headers(), signal });
  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(body.error ?? "The request could not be completed");
  }
  return (await response.json()) as T;
}

const personPath = (personId: string) => `/persons/${encodeURIComponent(personId)}/article`;

export function getPersonArticle(personId: string): Promise<Article> {
  return getJSON<Article>(personPath(personId));
}

/** The result of a save: saved (created false when nothing changed), a conflict carrying the
 * article as it now is, or refused (a body the schema refuses, or mentions of missing items). */
export type SaveResult =
  | { status: "saved"; article: Article; created: boolean }
  | { status: "conflict"; article: Article }
  | { status: "refused"; error: string; missing: { kind: MentionKind; ref: string }[] };

export async function savePersonArticle(
  personId: string,
  baseRevisionId: number | null,
  body: ArticleDoc,
  editSummary: string,
  templateId?: number,
): Promise<SaveResult> {
  const response = await apiFetch(`${apiBaseUrl}${personPath(personId)}`, {
    method: "PUT",
    headers: headers(),
    body: JSON.stringify({ baseRevisionId, body, editSummary, templateId: templateId ?? 0 }),
  });
  const data = (await response.json().catch(() => ({}))) as {
    error?: string;
    article?: Article;
    created?: boolean;
    missing?: { kind: MentionKind; ref: string }[];
  };
  if (response.ok && data.article) {
    return { status: "saved", article: data.article, created: data.created ?? false };
  }
  if (response.status === 409 && data.article) {
    return { status: "conflict", article: data.article };
  }
  if (response.status === 422 || response.status === 413) {
    return { status: "refused", error: data.error ?? "", missing: data.missing ?? [] };
  }
  throw new Error(data.error ?? "The article could not be saved");
}

/** A first draft generated from the records (articles.Template): logged on the server, not
 * saved as a revision until the analyst saves it. */
export interface ArticleTemplate {
  id: number;
  generator: string;
  language: string;
  generatedAt: string;
  body: ArticleDoc;
}

export async function generatePersonArticleTemplate(personId: string, language: string): Promise<ArticleTemplate> {
  const response = await apiFetch(`${apiBaseUrl}${personPath(personId)}/template`, {
    method: "POST",
    headers: headers(),
    body: JSON.stringify({ language }),
  });
  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(body.error ?? "The draft could not be generated");
  }
  return (await response.json()) as ArticleTemplate;
}

/** How much the records link to a person, by certainty (articles.LinkCounts). */
export interface LinkCounts {
  cases: { confirmed: number; unreviewed: number; uncertain: number; excluded: number };
  clusters: { reviewed: number; automatic: number };
  otherIdentities: { confirmed: number; unreviewed: number };
  associates: { confirmed: number; unreviewed: number };
}

export function getPersonLinkCounts(personId: string): Promise<LinkCounts> {
  return getJSON<LinkCounts>(`/persons/${encodeURIComponent(personId)}/link-counts`);
}

export async function getPersonArticleHistory(personId: string): Promise<ArticleRevision[]> {
  const data = await getJSON<{ revisions: ArticleRevision[] }>(`${personPath(personId)}/history`);
  return data.revisions;
}

export function getPersonArticleRevision(personId: string, number: number): Promise<ArticleRevision> {
  return getJSON<ArticleRevision>(`${personPath(personId)}/revisions/${number}`);
}

export async function getPersonBacklinks(personId: string): Promise<Backlink[]> {
  const data = await getJSON<{ backlinks: Backlink[] }>(`${personPath(personId)}/backlinks`);
  return data.backlinks;
}

export async function getCaseBacklinks(caseId: string): Promise<Backlink[]> {
  const data = await getJSON<{ backlinks: Backlink[] }>(
    `/cases/${encodeURIComponent(caseId)}/article-backlinks`,
  );
  return data.backlinks;
}

export async function searchMentions(
  q: string,
  kinds: MentionKind[],
  signal?: AbortSignal,
): Promise<MentionTarget[]> {
  const params = new URLSearchParams({ q });
  if (kinds.length > 0) params.set("kinds", kinds.join(","));
  const data = await getJSON<{ results: MentionTarget[] }>(`/mentions/search?${params}`, signal);
  return data.results;
}
