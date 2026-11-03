import StarterKit from "@tiptap/starter-kit";
import Mention, { type MentionOptions } from "@tiptap/extension-mention";
import { ReactNodeViewRenderer } from "@tiptap/react";
import type { Extensions } from "@tiptap/core";
import { MentionNodeView } from "./MentionChip";
import { mentionSuggestion } from "./suggestion";

/** A mention node carrying the target's kind, key and label (articles.Mention on the server). */
const ArticleMention = Mention.extend({
  addAttributes() {
    const attr = (name: string) => ({
      default: null,
      parseHTML: (element: HTMLElement) => element.getAttribute(`data-${name}`),
      renderHTML: (attributes: Record<string, unknown>) =>
        attributes[name] ? { [`data-${name}`]: String(attributes[name]) } : {},
    });
    return { kind: attr("kind"), ref: attr("ref"), label: attr("label") };
  },
  addNodeView() {
    return ReactNodeViewRenderer(MentionNodeView, { as: "span" });
  },
});

/** Only http(s) links, as the server allows (articles.checkHref). */
const isHttp = (url: string) => /^https?:\/\/[^\s/]/i.test(url);

/** The article schema (docs/person-article-plan.md §2): paragraphs, headings 2 and 3, lists,
 * quotes, hard breaks, bold, italic, http(s) links and mentions -- what the server accepts.
 * Everything else StarterKit offers is switched off, so it can neither be typed nor pasted. */
export function articleExtensions(editable: boolean): Extensions {
  return [
    StarterKit.configure({
      heading: { levels: [2, 3] },
      code: false,
      codeBlock: false,
      strike: false,
      underline: false,
      horizontalRule: false,
      link: {
        openOnClick: !editable,
        autolink: true,
        linkOnPaste: true,
        defaultProtocol: "https",
        protocols: [],
        isAllowedUri: (url) => isHttp(url),
        shouldAutoLink: (url) => isHttp(url) || /^[\w.-]+\.[a-z]{2,}/i.test(url),
        HTMLAttributes: { target: "_blank", rel: "noopener noreferrer nofollow" },
      },
    }),
    ArticleMention.configure({
      renderText: ({ node }) => `@${node.attrs.label ?? node.attrs.ref}`,
      // The mention's attributes are kind/ref/label rather than TipTap's id/label.
      suggestion: mentionSuggestion() as unknown as MentionOptions["suggestion"],
    }),
  ];
}

export { isHttp };
