import { useEffect, useState } from "react";
import { Link } from "react-router";
import { useTranslation } from "react-i18next";
import PageMeta from "../components/common/PageMeta";
import PageBreadcrumb from "../components/common/PageBreadCrumb";
import ComponentCard from "../components/common/ComponentCard";
import { Table, TableBody, TableCell, TableHeader, TableRow } from "../components/ui/table";
import Badge from "../components/ui/badge/Badge";
import { stageColor, typeKey } from "../components/analyses/labels";
import { listMyAnalyses, type AssignedAnalysis } from "../services/analyses";

const headerClass = "px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400";
const cellClass = "px-5 py-4 text-start text-theme-sm text-gray-700 dark:text-gray-300";

/** The signed-in examiner's work: every step of every analysis they hold, the ones waiting for
 * them first. */
export default function MyAnalyses() {
  const { t } = useTranslation();
  const [items, setItems] = useState<AssignedAnalysis[] | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    listMyAnalyses()
      .then((list) => setItems([...list].sort((a, b) => Number(b.yourTurn) - Number(a.yourTurn))))
      .catch((err) => setError(err instanceof Error ? err.message : t("analyses.loadError")));
  }, [t]);

  return (
    <>
      <PageMeta title={`${t("analyses.myTitle")} | TrackID`} description={t("analyses.metaDescription")} />
      <PageBreadcrumb pageTitle={t("analyses.myTitle")} />
      <ComponentCard title={t("analyses.myTitle")} desc={t("analyses.myDesc")}>
        {error && <p className="text-sm text-error-500">{error}</p>}
        {items && items.length === 0 && (
          <p className="text-sm text-gray-500 dark:text-gray-400">{t("analyses.myEmpty")}</p>
        )}
        {items && items.length > 0 && (
          <div className="overflow-x-auto">
            <Table>
              <TableHeader className="border-b border-gray-100 dark:border-white/[0.05]">
                <TableRow>
                  <TableCell isHeader className={headerClass}>{t("analyses.columns.case")}</TableCell>
                  <TableCell isHeader className={headerClass}>{t("analyses.columns.analysis")}</TableCell>
                  <TableCell isHeader className={headerClass}>{t("analyses.columns.yourStep")}</TableCell>
                  <TableCell isHeader className={headerClass}>{t("analyses.columns.stage")}</TableCell>
                  <TableCell isHeader className={headerClass}>{""}</TableCell>
                </TableRow>
              </TableHeader>
              <TableBody className="divide-y divide-gray-100 dark:divide-white/[0.05]">
                {items.map((item) => {
                  // A verification still to do is blind: no case, nothing to lead to it.
                  const blind = item.yourTurn && item.step === "REVIEWER";
                  return (
                    <TableRow key={`${item.analysisId}-${item.step}`}>
                      <TableCell className={cellClass}>
                        {blind ? (
                          <span className="text-gray-500">{t("analyses.blindCase")}</span>
                        ) : (
                          <>
                            <Link to={`/cases/${encodeURIComponent(item.caseId)}`} className="font-medium text-brand-500 hover:underline">
                              {item.caseId}
                            </Link>
                            <div className="text-theme-xs text-gray-500">{item.description}</div>
                          </>
                        )}
                      </TableCell>
                      <TableCell className={cellClass}>
                        {blind ? t(typeKey(item.type)) : `${t(typeKey(item.type))} #${item.sequence}`}
                      </TableCell>
                      <TableCell className={cellClass}>{t(`analyses.step.${item.step}`)}</TableCell>
                      <TableCell className={cellClass}>
                        <Badge size="sm" color={stageColor(item.stage, item.status)}>
                          {item.status === "CLOSED" ? t("analyses.closed") : t(`analyses.stage.${item.stage}`)}
                        </Badge>
                        {item.yourTurn && (
                          <Badge size="sm" color="primary">
                            {t("analyses.yourTurn")}
                          </Badge>
                        )}
                      </TableCell>
                      <TableCell className={cellClass}>
                        <Link
                          to={`/cases/${encodeURIComponent(item.caseId)}/analyses/${item.analysisId}`}
                          className="text-sm font-medium text-brand-500 hover:underline"
                        >
                          {item.yourTurn ? t("analyses.work") : t("analyses.view")}
                        </Link>
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          </div>
        )}
      </ComponentCard>
    </>
  );
}
