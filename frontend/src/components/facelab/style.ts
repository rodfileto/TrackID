import { useTranslation } from "react-i18next";
import type { Label } from "../../services/facelab";

// The face lab's shared classes and the label of a FISWG vocabulary entry in the interface's language.
export const inputClass =
  "rounded-lg border border-gray-300 bg-transparent px-2 py-1 text-base text-gray-800 shadow-theme-xs focus:border-brand-300 focus:outline-hidden dark:border-gray-700 dark:bg-gray-900 dark:text-white/90";

export const buttonClass = "rounded-lg border border-gray-300 px-3 py-1.5 text-sm text-gray-700 hover:bg-gray-50 disabled:opacity-50 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-white/5";

export const primaryClass = "rounded-lg bg-brand-500 px-3 py-1.5 text-sm font-medium text-white hover:bg-brand-600 disabled:opacity-50";

/** The label of a FISWG vocabulary entry in the interface's language. */
export function useLabel() {
  const { i18n } = useTranslation();
  const pt = (i18n.resolvedLanguage ?? i18n.language ?? "en").startsWith("pt");
  return (l?: Pick<Label, "en" | "pt">) => (l ? (pt ? l.pt : l.en) : "");
}

