import { useTranslation } from "react-i18next";
import { SUPPORTED_LANGUAGES } from "../../i18n";

export default function LanguageSwitcher() {
  const { i18n, t } = useTranslation();

  return (
    <select
      aria-label={t("language.label")}
      value={i18n.resolvedLanguage}
      onChange={(e) => i18n.changeLanguage(e.target.value)}
      className="h-11 rounded-full border border-gray-200 bg-white px-3 text-theme-sm text-gray-700 dark:border-gray-800 dark:bg-gray-900 dark:text-gray-400"
    >
      {SUPPORTED_LANGUAGES.map((l) => (
        <option key={l.code} value={l.code}>
          {l.label}
        </option>
      ))}
    </select>
  );
}
