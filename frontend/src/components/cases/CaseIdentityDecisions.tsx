import { useEffect, useState } from "react";
import { Link } from "react-router";
import { useTranslation } from "react-i18next";
import ComponentCard from "../common/ComponentCard";
import Badge from "../ui/badge/Badge";
import IdentityFileThumbnail from "../persons/IdentityFileThumbnail";
import {
  listCaseIdentityDecisions,
  type CaseFaceIdentities,
  type IdentityDecision,
} from "../../services/cases";
import type { CodificationThumbnail } from "../../hooks/useCodificationThumbnails";
import { codificationLabel } from "../../hooks/useCodificationThumbnails";

interface CaseIdentityDecisionsProps {
  caseId: string;
  faces: CodificationThumbnail[];
  /** Bumped when the case's faces change, to reload. */
  refreshKey: number;
}

const statusColor: Record<IdentityDecision["status"], "success" | "error" | "warning" | "light" | "info"> = {
  CONFIRMED: "success",
  EXCLUDED: "error",
  DISPUTED: "warning",
  INCONCLUSIVE: "info",
  PENDING: "light",
};

function StatusBadge({ decision }: { decision: IdentityDecision }) {
  const { t } = useTranslation();
  return (
    <div className="text-right">
      <Badge size="sm" color={statusColor[decision.status]}>
        {t(`caseIdentities.status.${decision.status}`)}
      </Badge>
      {decision.decidedBy && (
        <p className="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
          {t("caseIdentities.decidedBy", { name: decision.decidedBy })}
        </p>
      )}
    </div>
  );
}

function FaceIdentities({
  face,
  thumbnail,
}: {
  face: CaseFaceIdentities;
  thumbnail?: CodificationThumbnail;
}) {
  const { t } = useTranslation();
  return (
    <div className="rounded-xl border border-gray-200 p-4 dark:border-white/[0.05]">
      <div className="mb-3 flex items-center gap-2">
        {thumbnail && (
          <img
            src={thumbnail.thumbnailUrl}
            alt={codificationLabel(thumbnail.codification)}
            className="h-8 w-8 rounded-full object-cover"
          />
        )}
        <p className="text-sm font-medium text-gray-800 dark:text-white/90">
          {thumbnail
            ? t("caseIdentities.face", { label: codificationLabel(thumbnail.codification) })
            : t("caseIdentities.faceTrace", { id: face.traceId })}
        </p>
      </div>
      {face.identities.length === 0 ? (
        <p className="text-sm text-gray-400 dark:text-white/30">{t("caseIdentities.noCandidates")}</p>
      ) : (
        <ul className="divide-y divide-gray-100 rounded-lg border border-gray-200 dark:divide-white/[0.05] dark:border-white/[0.05]">
          {face.identities.map((d) => (
            <li key={d.featureId} className="flex items-center gap-3 px-3 py-2.5">
              <div className="shrink-0">
                <IdentityFileThumbnail
                  personId={d.personId}
                  identityFileId={d.identityFileId}
                  contentType={d.contentType}
                  alt={d.name}
                />
              </div>
              <div className="min-w-0 flex-1">
                <Link
                  to={`/persons/${encodeURIComponent(d.personId)}`}
                  className="font-medium text-brand-500 hover:text-brand-600 dark:text-brand-400 dark:hover:text-brand-300"
                >
                  {d.name}
                </Link>
                {d.score !== undefined && (
                  <p className="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                    {t("caseIdentities.score", { score: (d.score * 100).toFixed(1) })}
                  </p>
                )}
              </div>
              <div className="shrink-0">
                <StatusBadge decision={d} />
              </div>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

/** The case's decisions on enrolled identities: for each of its faces, every identity the
 * matcher reached at or above the review threshold (it writes nothing below it), and any an
 * examiner decided in a comparison, each with its state -- awaiting review, confirmed, excluded,
 * inconclusive or disputed (see cases.ListCaseIdentityDecisions). Unlike CaseClusters, which shows
 * only reviewed links, this is the review's own record. Nothing shows to the case's blind
 * verifier, nor for a case with no codified face. */
export default function CaseIdentityDecisions({ caseId, faces, refreshKey }: CaseIdentityDecisionsProps) {
  const { t } = useTranslation();
  const [data, setData] = useState<CaseFaceIdentities[] | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    let cancelled = false;
    setError("");
    listCaseIdentityDecisions(caseId)
      .then((res) => {
        if (!cancelled) setData(res.blind ? [] : res.faces);
      })
      .catch((err) => {
        if (!cancelled) setError(err instanceof Error ? err.message : t("caseIdentities.loadError"));
      });
    return () => {
      cancelled = true;
    };
  }, [caseId, refreshKey, t]);

  if (!error && (!data || data.length === 0)) return null;

  return (
    <ComponentCard title={t("caseIdentities.title")} desc={t("caseIdentities.desc")} collapsible>
      {error ? (
        <p className="py-8 text-center text-sm text-error-500">{error}</p>
      ) : (
        <div className="space-y-4">
          {data!.map((face) => (
            <FaceIdentities
              key={face.traceId}
              face={face}
              thumbnail={faces.find((f) => f.codification.traceId === face.traceId)}
            />
          ))}
        </div>
      )}
    </ComponentCard>
  );
}
