import { ReactRenderer } from "@tiptap/react";
import type { SuggestionOptions } from "@tiptap/suggestion";
import { searchMentions, type MentionTarget } from "../../services/articles";
import { parseMentionQuery } from "./mentionKinds";
import MentionMenu, { type MentionAttrs, type MentionMenuHandle, type MenuProps } from "./MentionMenu";

/** The longest text after "@" still searched: past it the menu gives up, so typing on after an
 * @ that was not a mention does not keep the menu open. */
const MAX_QUERY = 60;

/** The suggestion settings for the mention extension: what to search and how to show the menu,
 * placed under the caret. */
export function mentionSuggestion(): Omit<SuggestionOptions<MentionTarget, MentionAttrs>, "editor"> {
  let controller: AbortController | null = null;
  return {
    char: "@",
    allowSpaces: true,
    debounce: 150,
    items: async ({ query }) => {
      if (query.length > MAX_QUERY) return [];
      controller?.abort();
      controller = new AbortController();
      const { kinds, term } = parseMentionQuery(query);
      try {
        return await searchMentions(term, kinds, controller.signal);
      } catch {
        return [];
      }
    },
    render: () => {
      let component: ReactRenderer<MentionMenuHandle, MenuProps> | null = null;
      let dismissed = false; // Escape: stay closed until the next @
      const place = (props: MenuProps) => {
        const rect = props.clientRect?.();
        const element = component?.element as HTMLElement | undefined;
        if (!rect || !element) return;
        element.style.position = "fixed";
        element.style.zIndex = "100000";
        const below = rect.bottom + 6;
        const fitsBelow = below + 320 < window.innerHeight;
        element.style.top = fitsBelow ? `${below}px` : "";
        element.style.bottom = fitsBelow ? "" : `${window.innerHeight - rect.top + 6}px`;
        element.style.left = `${Math.max(8, Math.min(rect.left, window.innerWidth - 336))}px`;
      };
      const close = () => {
        component?.element.remove();
        component?.destroy();
        component = null;
      };
      return {
        onStart: (props) => {
          dismissed = false;
          component = new ReactRenderer(MentionMenu, { props, editor: props.editor });
          document.body.appendChild(component.element);
          place(props);
        },
        onUpdate: (props) => {
          if (props.query.length > MAX_QUERY) {
            close();
            return;
          }
          if (!component) {
            if (dismissed) return;
            component = new ReactRenderer(MentionMenu, { props, editor: props.editor });
            document.body.appendChild(component.element);
          }
          component.updateProps(props);
          place(props);
        },
        onKeyDown: (props) => {
          if (!component) return false;
          if (props.event.key === "Escape") {
            dismissed = true;
            close();
            return true;
          }
          return component?.ref?.onKeyDown(props) ?? false;
        },
        onExit: close,
      };
    },
  };
}
