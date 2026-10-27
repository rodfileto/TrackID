import { useState } from "react";
import { Link } from "react-router";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import ComponentCard from "../common/ComponentCard";
import Badge from "../ui/badge/Badge";
import Button from "../ui/button/Button";
import CaseTraceThumbnail from "./CaseTraceThumbnail";
import IdentityFileThumbnail from "../persons/IdentityFileThumbnail";
import { useCurrentUser } from "../../context/CurrentUserContext";
import { openAnalysis } from "../../services/analyses";
import type { CaseCluster, PathStep, TraceLink } from "../../services/cases";
import type { CodificationThumbnail } from "../../hooks/useCodificationThumbnails";
import { codificationLabel } from "../../hooks/useCodificationThumbnails";
import LinkBadge from "../analyses/LinkBadge";

interface CaseClustersProps {
  caseId: string;
  /** A direct comparison was opened from one of the links. */
  onAnalysisOpened?: () => void;
  clusters: CaseCluster[];
  faces: CodificationThumbnail[];
  loading: boolean;
  error: string;
}

function modalityLabelKey(modality: string): string {
  return modality === "FACIAL" ? "modality.facial" : "modality.fingerprint";
}

function modalityColor(modality: string): "info" | "warning" {
  return modality === "FACIAL" ? "info" : "warning";
}

function stepLabel(step: PathStep): string {
  return step.kind === "KNOWN" ? step.name || step.personId || "" : step.caseId || "";
}

/** How a link was confirmed: a direct link names who settled its decision; an indirect one is
 * marked as such, with the samples its path runs through. Every link here is reviewed -- the case
 * page never shows an automatic (SYSTEM-only) match. */
function DecisionBadge({ link }: { link: TraceLink }) {
  const { t } = useTranslation();
  if (!link.direct) {
    const path = (link.path ?? []).map(stepLabel).join(" → ");
    return (
      <div className="text-right" title={t("caseClusters.indirectHint")}>
        <Badge size="sm" color="light">
          {t("caseClusters.indirect")}
        </Badge>
        {path && (
          <p className="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
            {t("caseClusters.via", { path })}
          </p>
        )}
      </div>
    );
  }
  return (
    <div className="flex flex-col items-end gap-1">
      <Badge size="sm" color="success">
        {t("caseClusters.confirmedBy", { name: link.decidedBy || t("caseClusters.anExaminer") })}
        {link.confidence !== undefined &&
          ` · ${(link.confidence * 100).toFixed(1)}%`}
      </Badge>
      <LinkBadge link={link.link} />
    </div>
  );
}

/** Admin, on an indirect link to an enrolled person: open a FACIAL_COMPARISON of the case's face
 * the path starts from against that person, so the link can be confirmed directly. A comparison
 * with another case's face cannot be opened yet (pairs across cases are not distributed). */
function RequestComparison({
  caseId,
  link,
  onOpened,
}: {
  caseId: string;
  link: TraceLink;
  onOpened?: () => void;
}) {
  const { t } = useTranslation();
  const [busy, setBusy] = useState(false);
  if (link.kind !== "KNOWN") {
    return (
      <p className="mt-1 text-right text-xs text-gray-400 dark:text-white/30">
        {t("caseClusters.crossCaseHint")}
      </p>
    );
  }
  if (!link.featureId || !link.fromFeatureId) return null;

  async function request() {
    setBusy(true);
    try {
      await openAnalysis(caseId, "FACIAL_COMPARISON", [
        { featureA: link.fromFeatureId!, featureB: link.featureId! },
      ]);
      toast.success(t("caseClusters.comparisonOpened"));
      onOpened?.();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("caseClusters.comparisonError"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Button size="sm" variant="outline" className="mt-1.5" disabled={busy} onClick={request}>
      {t("caseClusters.requestComparison")}
    </Button>
  );
}

function LinkRow({
  link,
  caseId,
  onAnalysisOpened,
}: {
  link: TraceLink;
  caseId: string;
  onAnalysisOpened?: () => void;
}) {
  const { t } = useTranslation();
  const { isCoordinator } = useCurrentUser();
  return (
    <li className="flex items-center gap-3 px-3 py-2.5">
      <div className="shrink-0">
        {link.kind === "QUESTIONED" ? (
          <CaseTraceThumbnail
            caseId={link.caseId ?? ""}
            thumbnailFileId={link.thumbnailFileId}
            thumbnailBox={link.thumbnailBox}
            alt={t("personLookup.traceAlt", { traceId: link.traceId, caseId: link.caseId })}
          />
        ) : (
          <IdentityFileThumbnail
            personId={link.personId ?? ""}
            identityFileId={link.identityFileId ?? 0}
            contentType={link.contentType}
            alt={link.name ?? ""}
          />
        )}
      </div>

      <div className="min-w-0 flex-1">
        {link.kind === "QUESTIONED" ? (
          <>
            <div className="flex items-center gap-2">
              <Link
                to={`/cases/${encodeURIComponent(link.caseId ?? "")}`}
                className="font-medium text-brand-500 hover:text-brand-600 dark:text-brand-400 dark:hover:text-brand-300"
              >
                {link.caseId}
              </Link>
              {link.modality && (
                <Badge size="sm" color={modalityColor(link.modality)}>
                  {t(modalityLabelKey(link.modality))}
                </Badge>
              )}
            </div>
            <p className="mt-0.5 truncate text-xs text-gray-500 dark:text-gray-400">
              {link.description || t("caseDetail.noDescription")}
            </p>
          </>
        ) : (
          <>
            <Link
              to={`/persons/${encodeURIComponent(link.personId ?? "")}`}
              className="font-medium text-brand-500 hover:text-brand-600 dark:text-brand-400 dark:hover:text-brand-300"
            >
              {link.name}
            </Link>
            <p className="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
              {t("caseClusters.enrolledIdentity")}
            </p>
          </>
        )}
      </div>

      <div className="flex shrink-0 flex-col items-end">
        <DecisionBadge link={link} />
        {isCoordinator && !link.direct && (
          <RequestComparison caseId={caseId} link={link} onOpened={onAnalysisOpened} />
        )}
      </div>
    </li>
  );
}

function LocalFaces({
  traceIds,
  faces,
}: {
  traceIds: number[];
  faces: CodificationThumbnail[];
}) {
  const items = traceIds
    .map((traceId) => faces.find((f) => f.codification.traceId === traceId))
    .filter((f): f is CodificationThumbnail => !!f);

  if (items.length === 0) return null;

  return (
    <div className="flex flex-wrap items-center gap-2">
      {items.map((face) => (
        <span
          key={face.codification.id}
          title={codificationLabel(face.codification)}
          className="flex items-center gap-1.5 rounded-full border border-gray-200 bg-white py-0.5 pl-0.5 pr-2 text-xs text-gray-600 dark:border-white/[0.05] dark:bg-white/[0.03] dark:text-gray-300"
        >
          <img
            src={face.thumbnailUrl}
            alt={codificationLabel(face.codification)}
            className="h-5 w-5 rounded-full object-cover"
          />
          {codificationLabel(face.codification)}
        </span>
      ))}
    </div>
  );
}

function ClusterCard({
  cluster,
  faces,
  caseId,
  onAnalysisOpened,
}: {
  cluster: CaseCluster;
  faces: CodificationThumbnail[];
  caseId: string;
  onAnalysisOpened?: () => void;
}) {
  const { t } = useTranslation();
  const cases = (cluster.links ?? []).filter((l) => l.kind === "QUESTIONED");
  const persons = (cluster.links ?? []).filter((l) => l.kind === "KNOWN");

  return (
    <div className="rounded-xl border border-gray-200 p-4 dark:border-white/[0.05]">
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <p className="text-sm font-medium text-gray-800 dark:text-white/90">
          {t("personProfile.clusterNumber", { id: cluster.clusterId })}
        </p>
        <p className="text-xs text-gray-500 dark:text-gray-400">
          {t("caseClusters.samples", { count: cluster.memberCount })}
        </p>
      </div>

      <div className="mb-4">
        <p className="mb-1.5 text-xs font-medium text-gray-500 dark:text-gray-400">
          {t("caseClusters.localFaces")}
        </p>
        <LocalFaces traceIds={cluster.localTraceIds} faces={faces} />
      </div>

      <div className="mb-4">
        <p className="mb-1.5 text-xs font-medium text-gray-500 dark:text-gray-400">
          {t("caseClusters.relatedCases")}
        </p>
        {cases.length === 0 ? (
          <p className="text-sm text-gray-400 dark:text-white/30">
            {t("caseClusters.noOtherEvidence")}
          </p>
        ) : (
          <ul className="divide-y divide-gray-100 rounded-lg border border-gray-200 dark:divide-white/[0.05] dark:border-white/[0.05]">
            {cases.map((link) => (
              <LinkRow
                key={`${link.caseId}-${link.traceId}`}
                link={link}
                caseId={caseId}
                onAnalysisOpened={onAnalysisOpened}
              />
            ))}
          </ul>
        )}
      </div>

      <div>
        <p className="mb-1.5 text-xs font-medium text-gray-500 dark:text-gray-400">
          {t("caseClusters.identifiedPersons")}
        </p>
        {persons.length === 0 ? (
          <p className="text-sm text-gray-400 dark:text-white/30">
            {t("caseClusters.notResolved")}
          </p>
        ) : (
          <ul className="divide-y divide-gray-100 rounded-lg border border-gray-200 dark:divide-white/[0.05] dark:border-white/[0.05]">
            {persons.map((link) => (
              <LinkRow
                key={link.featureId ?? link.personId}
                link={link}
                caseId={caseId}
                onAnalysisOpened={onAnalysisOpened}
              />
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}

/** The distinct REVIEWED biometric clusters this case's faces have resolved
 * into -- merged by cluster, not repeated per face (see
 * cases.ListCaseClusters) -- plus the other criminal cases and enrolled
 * persons each one is linked to, directly or through a path of reviewed
 * links. Automatic matches never show here, not even as a count: they are for
 * the intelligence views (the Clusters page). */
export default function CaseClusters({
  caseId,
  onAnalysisOpened,
  clusters,
  faces,
  loading,
  error,
}: CaseClustersProps) {
  const { t } = useTranslation();
  // Nothing to show until a reviewed decision links one of the case's faces to another sample:
  // no panel at all, rather than an empty one.
  if (!error && clusters.length === 0) return null;
  return (
    <ComponentCard
      title={t("caseClusters.title")}
      collapsible
      desc={
        loading
          ? t("common.loading")
          : `${t("caseClusters.reached", { count: clusters.length })} ${t("caseClusters.reviewedOnly")}`
      }
    >
      {loading && (
        <p className="py-8 text-center text-sm text-gray-500 dark:text-gray-400">
          {t("common.loading")}
        </p>
      )}

      {!loading && error && (
        <p className="py-8 text-center text-sm text-error-500">{error}</p>
      )}

      {!loading && !error && clusters.length > 0 && (
        <div className="space-y-4">
          {clusters.map((cluster) => (
            <ClusterCard
              key={cluster.clusterId}
              cluster={cluster}
              faces={faces}
              caseId={caseId}
              onAnalysisOpened={onAnalysisOpened}
            />
          ))}
        </div>
      )}
    </ComponentCard>
  );
}
