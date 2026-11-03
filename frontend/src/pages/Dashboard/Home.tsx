import { Link } from "react-router";
import { useTranslation } from "react-i18next";
import ClusterGraph from "../../components/dashboard/ClusterGraph";
import PageMeta from "../../components/common/PageMeta";
import ComponentCard from "../../components/common/ComponentCard";
import { useCurrentUser } from "../../context/CurrentUserContext";

/** The landing page: the way to the user's own work, and an anonymous illustration of how
 * identities and case traces connect. */
export default function Home() {
  const { t } = useTranslation();
  const { user, isExaminer, isCoordinator } = useCurrentUser();
  const links = [
    isExaminer && { to: "/analyses", label: t("nav.myAnalyses") },
    { to: "/cases", label: t("nav.criminalCases") },
    isCoordinator && { to: "/requests", label: t("nav.requests") },
    { to: "/image-lab", label: t("nav.imageLab") },
  ].filter(Boolean) as { to: string; label: string }[];
  return (
    <>
      <PageMeta title={`${t("nav.dashboard")} | TrackID`} description={t("dashboard.metaDescription")} />
      <div className="grid grid-cols-12 gap-4 md:gap-6">
        <div className="col-span-12">
          <ComponentCard
            title={t("dashboard.welcome", { name: user?.first_name ?? "" })}
            desc={t("dashboard.welcomeDesc")}
          >
            <div className="flex flex-wrap gap-3">
              {links.map((l) => (
                <Link key={l.to} to={l.to} className="inline-flex h-10 items-center rounded-lg border border-gray-300 px-4 text-sm font-medium text-gray-700 hover:border-brand-300 hover:text-brand-600 dark:border-gray-700 dark:text-gray-300">
                  {l.label}
                </Link>
              ))}
            </div>
          </ComponentCard>
        </div>
        <div className="col-span-12">
          <ClusterGraph />
        </div>
      </div>
    </>
  );
}
