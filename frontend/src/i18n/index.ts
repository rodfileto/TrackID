import i18n from "i18next";
import LanguageDetector from "i18next-browser-languagedetector";
import { initReactI18next } from "react-i18next";
import en from "./locales/en.json";
import ptBR from "./locales/pt-BR.json";

export const SUPPORTED_LANGUAGES = [
  { code: "en", label: "English" },
  { code: "pt-BR", label: "Português (BR)" },
] as const;

i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    resources: {
      en: { translation: en },
      "pt-BR": { translation: ptBR },
    },
    supportedLngs: SUPPORTED_LANGUAGES.map((l) => l.code),
    fallbackLng: "en",
    interpolation: { escapeValue: false },
    detection: {
      order: ["localStorage", "navigator"],
      lookupLocalStorage: "trackid.lang",
      caches: ["localStorage"],
      // Any Portuguese variant (pt, pt-PT, ...) gets the pt-BR translations.
      convertDetectedLanguage: (lng: string) =>
        lng.startsWith("pt") ? "pt-BR" : lng,
    },
  });

const syncHtmlLang = (lng: string) => {
  document.documentElement.lang = lng;
};
syncHtmlLang(i18n.resolvedLanguage ?? "en");
i18n.on("languageChanged", syncHtmlLang);

export default i18n;
