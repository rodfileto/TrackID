import { Outlet } from "react-router";
import { useTranslation } from "react-i18next";
import { useCurrentUser } from "../../context/CurrentUserContext";

/** A capability a page needs (docs/access-profiles-plan.md); "persons" is examiners and analysts. */
export type Capability = "coordinator" | "intelligence" | "persons";

/** Shows its routes only to users with the capability, and a short explanation to anyone else, so
 * a direct link does not open a page whose every request the server would refuse. */
export default function RequireCapability({ need }: { need: Capability }) {
  const { t } = useTranslation();
  const { isExaminer, isIntelligence, isCoordinator } = useCurrentUser();
  const allowed =
    (need === "coordinator" && isCoordinator) ||
    (need === "intelligence" && isIntelligence) ||
    (need === "persons" && (isExaminer || isIntelligence));
  if (allowed) return <Outlet />;
  return (
    <div className="rounded-2xl border border-gray-200 bg-white p-6 dark:border-gray-800 dark:bg-white/[0.03]">
      <h2 className="text-lg font-semibold text-gray-800 dark:text-white/90">{t("access.deniedTitle")}</h2>
      <p className="mt-2 text-sm text-gray-500 dark:text-gray-400">{t(`access.denied.${need}`)}</p>
    </div>
  );
}
