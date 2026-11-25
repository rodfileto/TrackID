import { Outlet } from "react-router";
import { useTranslation } from "react-i18next";
import { useCurrentUser, type Need } from "../../context/CurrentUserContext";

/** A capability a page needs (docs/access-profiles-plan.md); "persons" is examiners and analysts,
 * "admin" account management, "scene" registering a scene examined on call (coordinators and
 * fingerprint examiners, docs/scene-collection-plan.md). */
export type Capability = "admin" | "coordinator" | "intelligence" | "persons" | "scene";

/** Shows its routes only to users with the capability, or the extension's grant, and a short
 * explanation to anyone else, so a direct link does not open a page whose every request the
 * server would refuse. A grant's page shows nothing while the grants are still being read. */
export default function RequireCapability({ need }: { need: Need }) {
  const { t } = useTranslation();
  const { can, grants } = useCurrentUser();
  if (can(need)) return <Outlet />;
  if (typeof need === "object" && grants === null) return null;
  return (
    <div className="rounded-2xl border border-gray-200 bg-white p-6 dark:border-gray-800 dark:bg-white/[0.03]">
      <h2 className="text-lg font-semibold text-gray-800 dark:text-white/90">{t("access.deniedTitle")}</h2>
      <p className="mt-2 text-sm text-gray-500 dark:text-gray-400">
        {t(typeof need === "object" ? "access.denied.grant" : `access.denied.${need}`)}
      </p>
    </div>
  );
}
