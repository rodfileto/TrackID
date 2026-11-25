import { useTranslation } from "react-i18next";
import { useLocation } from "react-router";
import { previewProfiles, useCurrentUser } from "../../context/CurrentUserContext";

/** "View as", for admins on `/`: shows the home page and the menu as another profile would see them. */
export default function PreviewSwitcher() {
  const { t } = useTranslation();
  const { pathname } = useLocation();
  const { canPreview, preview, setPreview } = useCurrentUser();
  if (!canPreview || pathname !== "/") return null;
  return (
    <select
      aria-label={t("preview.label")}
      title={t("preview.label")}
      value={preview ? previewProfiles.indexOf(preview) : -1}
      onChange={(e) => {
        const index = Number(e.target.value);
        setPreview(index < 0 ? null : previewProfiles[index]);
      }}
      className={`h-11 rounded-full border px-3 text-theme-sm ${
        preview
          ? "border-warning-300 bg-warning-50 text-warning-700 dark:border-warning-500/40 dark:bg-warning-500/15 dark:text-warning-400"
          : "border-gray-200 bg-white text-gray-700 dark:border-gray-800 dark:bg-gray-900 dark:text-gray-400"
      }`}
    >
      <option value={-1}>{t("preview.myself")}</option>
      {previewProfiles.map((profile, index) => (
        <option key={profile.name} value={index}>
          {t("preview.as", { profile: t(profile.name) })}
        </option>
      ))}
    </select>
  );
}
