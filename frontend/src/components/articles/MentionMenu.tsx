import { forwardRef, useEffect, useImperativeHandle, useState } from "react";
import { useTranslation } from "react-i18next";
import type { SuggestionKeyDownProps, SuggestionProps } from "@tiptap/suggestion";
import { MENTION_KINDS, type MentionTarget } from "../../services/articles";
import { KIND_CLASS, KIND_CODE, parseMentionQuery } from "./mentionKinds";
import { mentionKey, pickedTargets } from "./mentionContext";

/** The attributes a mention node stores: the target and its label when inserted. */
export interface MentionAttrs {
  kind: string;
  ref: string;
  label: string;
}

export type MenuProps = SuggestionProps<MentionTarget, MentionAttrs>;

export interface MentionMenuHandle {
  onKeyDown: (props: SuggestionKeyDownProps) => boolean;
}

/** The @ menu: results grouped by kind, arrow keys to move, Enter or Tab to insert. */
const MentionMenu = forwardRef<MentionMenuHandle, MenuProps>(function MentionMenu({ items, command, query }, ref) {
  const { t, i18n } = useTranslation();
  const [selected, setSelected] = useState(0);
  useEffect(() => setSelected(0), [items]);

  const choose = (index: number) => {
    const target = items[index];
    if (!target) return;
    pickedTargets.set(mentionKey(target.kind, target.ref), target);
    command({ kind: target.kind, ref: target.ref, label: target.label });
  };

  useImperativeHandle(ref, () => ({
    onKeyDown: ({ event }) => {
      if (items.length === 0) return false;
      if (event.key === "ArrowDown") {
        setSelected((s) => (s + 1) % items.length);
        return true;
      }
      if (event.key === "ArrowUp") {
        setSelected((s) => (s + items.length - 1) % items.length);
        return true;
      }
      if (event.key === "Enter" || event.key === "Tab") {
        choose(selected);
        return true;
      }
      return false;
    },
  }));

  const { kinds } = parseMentionQuery(query);
  const locale = i18n.resolvedLanguage ?? "en";
  let index = -1;
  return (
    <div className="max-h-80 w-80 overflow-y-auto rounded-xl border border-gray-200 bg-white p-1 text-sm shadow-theme-lg dark:border-gray-700 dark:bg-gray-900">
      {items.length === 0 && (
        <p className="px-3 py-2 text-gray-500 dark:text-gray-400">
          {query.trim() === "" ? t("articles.menu.typeToSearch") : t("articles.menu.noMatches")}
        </p>
      )}
      {MENTION_KINDS.filter((kind) => items.some((item) => item.kind === kind)).map((kind) => (
        <div key={kind} className="py-1">
          <p className="px-3 pb-1 text-[10px] font-semibold uppercase tracking-wide text-gray-400">
            {t(`articles.kindPlural.${kind}`)}
          </p>
          {items
            .filter((item) => item.kind === kind)
            .map((item) => {
              index += 1;
              const i = index;
              return (
                <button
                  key={`${item.kind}:${item.ref}`}
                  type="button"
                  onMouseDown={(e) => e.preventDefault()}
                  onClick={() => choose(i)}
                  onMouseEnter={() => setSelected(i)}
                  className={`flex w-full items-start gap-2 rounded-lg px-3 py-1.5 text-left ${
                    i === selected ? "bg-gray-100 dark:bg-white/10" : ""
                  }`}
                >
                  <span className={`mt-0.5 rounded px-1 text-[10px] font-semibold ${KIND_CLASS[item.kind]}`}>
                    {KIND_CODE[item.kind]}
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="block truncate font-medium text-gray-800 dark:text-white/90">{item.label}</span>
                    <span className="block truncate text-xs text-gray-500 dark:text-gray-400">
                      {[item.description !== item.label ? item.description : "", item.date ? new Date(item.date).toLocaleDateString(locale) : ""]
                        .filter(Boolean)
                        .join(" · ")}
                    </span>
                  </span>
                </button>
              );
            })}
        </div>
      ))}
      {kinds.length === 0 && (
        <p className="border-t border-gray-100 px-3 pt-1.5 pb-1 text-[11px] text-gray-400 dark:border-gray-800">
          {t("articles.menu.prefixHint")}
        </p>
      )}
    </div>
  );
});

export default MentionMenu;
