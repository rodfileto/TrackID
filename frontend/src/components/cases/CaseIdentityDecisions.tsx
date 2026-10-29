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
import LinkBadge from "../analyses/LinkBadge";
import Button from "../ui/button/Button";
import { toast } from "sonner";
import { useCurrentUser } from "../../context/CurrentUserContext";
import { addToComparison } from "../../services/analyses";

interface CaseIdentityDecisionsProps {
  caseId: string;
  /** FACIAL or FINGERPRINT: how traces are named and scores shown. */
  modality?: string;
  faces: CodificationThumbnail[];
  /** Bumped when the case's faces change, to reload. */
  refreshKey: number;
  /** Called after the coordinator adds a candidate to a comparison. */
  onPairAdded?: () => void;
}

/** Coordinator: add a candidate the matcher proposed (an enrolled person) to the case's
 * comparison -- the one its request opened, or a new one. */
function AddToComparison({ caseId, modality, traceId, featureId, onAdded }: {
  caseId: string;
  modality: string;
  traceId: number;
  featureId: string;
  onAdded?: () => void;
}) {
  const { t } = useTranslation();
  const [busy, setBusy] = useState(false);
  async function add() {
    setBusy(true);
    try {
      const { added } = await addToComparison(caseId, modality, [{ featureA: `TRACE:${traceId}#feature`, featureB: featureId }]);
      toast.success(added ? t("analyses.pairsAdded", { count: 1 }) : t("analyses.comparisonOpened"));
      onAdded?.();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("analyses.saveError"));
    } finally {
      setBusy(false);
    }
  }
  return (
    <Button size="sm" variant="outline" className="mt-1.5" disabled={busy} onClick={() => void add()}>
      {t("caseIdentities.addToComparison")}
    </Button>
  );
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
      {decision.link ? (
        <div className="mt-1">
          <LinkBadge link={decision.link} />
        </div>
      ) : (
        decision.conclusion && (
          <p className="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{t(`analyses.conclusion.${decision.conclusion}`)}</p>
        )
      )}
      {decision.decidedBy && (
        <p className="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
          {t("caseIdentities.decidedBy", { name: decision.decidedBy })}
        </p>
      )}
    </div>
  );
}

function FaceIdentities({
  caseId,
  modality,
  face,
  thumbnail,
  onPairAdded,
}: {
  caseId: string;
  modality: string;
  face: CaseFaceIdentities;
  thumbnail?: CodificationThumbnail;
  onPairAdded?: () => void;
}) {
  const { t } = useTranslation();
  const { isCoordinator } = useCurrentUser();
  const fingerprint = modality === "FINGERPRINT";
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
          {fingerprint
            ? t("caseIdentities.fragment", { id: face.traceId })
            : thumbnail
              ? t("caseIdentities.face", { label: codificationLabel(thumbnail.codification) })
              : t("caseIdentities.faceTrace", { id: face.traceId })}
        </p>
        {face.role === "REFERENCE" && (
          <Badge size="sm" color="info">
            {t("requests.role.REFERENCE")}
          </Badge>
        )}
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
                    {fingerprint
                      ? t("caseIdentities.matcherScore", { score: d.score.toFixed(1) })
                      : t("caseIdentities.score", { score: (d.score * 100).toFixed(1) })}
                  </p>
                )}
              </div>
              <div className="flex shrink-0 flex-col items-end">
                <StatusBadge decision={d} />
                {isCoordinator && face.role !== "REFERENCE" && (d.status === "PENDING" || d.status === "INCONCLUSIVE") && (
                  <AddToComparison caseId={caseId} modality={modality} traceId={face.traceId} featureId={d.featureId} onAdded={onPairAdded} />
                )}
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
export default function CaseIdentityDecisions({ caseId, modality = "FACIAL", faces, refreshKey, onPairAdded }: CaseIdentityDecisionsProps) {
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
    <ComponentCard
      title={t("caseIdentities.title")}
      desc={t(modality === "FINGERPRINT" ? "caseIdentities.descFingerprint" : "caseIdentities.desc")}
      collapsible
    >
      {error ? (
        <p className="py-8 text-center text-sm text-error-500">{error}</p>
      ) : (
        <div className="space-y-4">
          {data!.map((face) => (
            <FaceIdentities
              key={face.traceId}
              caseId={caseId}
              modality={modality}
              onPairAdded={onPairAdded}
              face={face}
              thumbnail={faces.find((f) => f.codification.traceId === face.traceId)}
            />
          ))}
        </div>
      )}
    </ComponentCard>
  );
}
