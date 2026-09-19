import { useTranslation } from "react-i18next";
import ClusterGraph from "../../components/dashboard/ClusterGraph";
import PageMeta from "../../components/common/PageMeta";

export default function Home() {
  const { t } = useTranslation();
  return (
    <>
      <PageMeta
        title={`${t("nav.dashboard")} | TrackID`}
        description={t("dashboard.metaDescription")}
      />
      <div className="grid grid-cols-12 gap-4 md:gap-6">
        <div className="col-span-12">
          <ClusterGraph />
        </div>
      </div>
    </>
  );
}
