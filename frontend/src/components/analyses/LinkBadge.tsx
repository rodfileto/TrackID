import { useTranslation } from "react-i18next";
import Badge from "../ui/badge/Badge";
import type { LinkStrength } from "../../services/analyses";

/** How strong a reviewed link is: identified by a lead, identified by a laudo at its level, or
 * only supported by a laudo below the identification level -- with a later comparison that left a
 * lead standing. */
export default function LinkBadge({ link }: { link?: LinkStrength }) {
  const { t } = useTranslation();
  if (!link) return null;
  const level = link.conclusion ? t(`analyses.conclusion.${link.conclusion}`) : "";
  const label =
    link.basis === "LEAD"
      ? t("links.LEAD")
      : link.identified
        ? t("links.LAUDO_IDENTIFIED", { level })
        : t("links.LAUDO_SUPPORTED", { level });
  const color = link.basis === "LAUDO" ? (link.identified ? "success" : "info") : "warning";
  return (
    <span className="inline-flex flex-wrap items-center gap-1">
      <Badge size="sm" color={color}>
        {label}
      </Badge>
      {link.laterLaudo && (
        <Badge size="sm" color="light">
          {t(`links.laterLaudo.${link.laterLaudo}`)}
        </Badge>
      )}
    </span>
  );
}
