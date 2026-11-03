import type { MentionKind, MentionTarget } from "../../services/articles";

/** Prefixes that narrow the @ search to one kind ("@ipl 4512", "@caso 2026"). Both languages'
 * words are accepted whatever the interface language, so a habit carries over. */
const PREFIXES: Record<string, MentionKind> = {
  case: "CASE",
  caso: "CASE",
  bo: "OCCURRENCE",
  occurrence: "OCCURRENCE",
  ocorrencia: "OCCURRENCE",
  ocorrência: "OCCURRENCE",
  ipl: "INQUIRY",
  inquiry: "INQUIRY",
  inquerito: "INQUIRY",
  inquérito: "INQUIRY",
  person: "PERSON",
  pessoa: "PERSON",
  cluster: "CLUSTER",
  agrupamento: "CLUSTER",
  request: "REQUEST",
  oficio: "REQUEST",
  ofício: "REQUEST",
  requisicao: "REQUEST",
  requisição: "REQUEST",
  report: "REPORT",
  laudo: "REPORT",
};

/** Splits what was typed after "@" into the kinds to search and the term: "ipl 4512" searches
 * inquiries for "4512"; "souza" searches every kind. A prefix alone ("ipl") lists the newest of
 * that kind. */
export function parseMentionQuery(query: string): { kinds: MentionKind[]; term: string } {
  const trimmed = query.trimStart();
  const match = /^(\S+)(?:\s+(.*))?$/u.exec(trimmed);
  if (match) {
    const kind = PREFIXES[match[1].toLowerCase()];
    if (kind && (match[2] !== undefined || trimmed === match[1])) {
      return { kinds: [kind], term: (match[2] ?? "").trim() };
    }
  }
  return { kinds: [], term: trimmed.trim() };
}

/** A short code shown at the start of each chip, so the kind reads at a glance. */
export const KIND_CODE: Record<MentionKind, string> = {
  CASE: "CASO",
  OCCURRENCE: "BO",
  INQUIRY: "IPL",
  PERSON: "P",
  CLUSTER: "CL",
  REQUEST: "OF",
  REPORT: "LAU",
};

/** Chip colours per kind (light and dark). */
export const KIND_CLASS: Record<MentionKind, string> = {
  CASE: "bg-brand-50 text-brand-700 dark:bg-brand-500/15 dark:text-brand-300",
  OCCURRENCE: "bg-orange-50 text-orange-700 dark:bg-orange-500/15 dark:text-orange-300",
  INQUIRY: "bg-amber-50 text-amber-800 dark:bg-amber-500/15 dark:text-amber-300",
  PERSON: "bg-success-50 text-success-700 dark:bg-success-500/15 dark:text-success-300",
  CLUSTER: "bg-blue-light-50 text-blue-light-700 dark:bg-blue-light-500/15 dark:text-blue-light-300",
  REQUEST: "bg-gray-100 text-gray-700 dark:bg-white/10 dark:text-gray-300",
  REPORT: "bg-theme-purple-500/10 text-theme-purple-500 dark:bg-theme-purple-500/20 dark:text-purple-300",
};

/** Where a chip leads, or null when the target has no page yet (occurrences and inquiries get
 * theirs in phase 4 of docs/person-article-plan.md). */
export function targetHref(target: MentionTarget): string | null {
  switch (target.kind) {
    case "CASE":
      return `/cases/${encodeURIComponent(target.ref)}`;
    case "PERSON":
      return `/persons/${encodeURIComponent(target.ref)}`;
    case "CLUSTER":
      return "/clusters";
    case "REQUEST":
    case "REPORT":
      return target.caseIds && target.caseIds.length > 0
        ? `/cases/${encodeURIComponent(target.caseIds[0])}`
        : null;
    default:
      return null;
  }
}
