import { useTranslation } from "react-i18next";
import PageMeta from "../components/common/PageMeta";
import PageBreadcrumb from "../components/common/PageBreadCrumb";
import ComponentCard from "../components/common/ComponentCard";
import Badge from "../components/ui/badge/Badge";

/** An examination requests will route to later (docs/request-routing-plan.md): what it will be,
 * and that it is not available yet. `kind` names its i18n texts under comingSoon. */
export default function ComingSoon({ kind }: { kind: "laboratory" | "crimeScene" }) {
  const { t } = useTranslation();
  const title = t(`comingSoon.${kind}.title`);
  return (
    <>
      <PageMeta title={`${title} | TrackID`} description={t(`comingSoon.${kind}.desc`)} />
      <PageBreadcrumb pageTitle={title} />
      <ComponentCard title={title}>
        <div className="flex flex-col items-start gap-4">
          <Badge size="sm" color="warning">
            {t("comingSoon.label")}
          </Badge>
          <p className="max-w-2xl text-sm text-gray-700 dark:text-gray-300">{t(`comingSoon.${kind}.desc`)}</p>
          <p className="max-w-2xl text-sm text-gray-500 dark:text-gray-400">{t("comingSoon.note")}</p>
        </div>
      </ComponentCard>
    </>
  );
}
