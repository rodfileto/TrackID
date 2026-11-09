import { createContext } from "react";
import type { MentionTarget, ResolvedMention } from "../../services/articles";

/** What a chip needs from the page: each mention as it reads now (from the server), and
 * whether the article is being edited (chips then do not navigate). */
export interface MentionContextValue {
  resolved: Map<string, ResolvedMention>;
  editing: boolean;
}

export const MentionContext = createContext<MentionContextValue>({
  resolved: new Map(),
  editing: false,
});

export const mentionKey = (kind: string, ref: string) => `${kind}:${ref}`;

/** Targets picked from the @ menu during this session, so a chip inserted before the next save
 * can show its card before the server has resolved it. */
export const pickedTargets = new Map<string, MentionTarget>();

export function resolvedMap(mentions: ResolvedMention[]): Map<string, ResolvedMention> {
  return new Map(mentions.map((m) => [mentionKey(m.kind, m.ref), m]));
}
