import { useEffect, useState } from "react";
import { Link, useNavigate, useParams, useSearchParams } from "react-router";
import BackLink from "../components/common/BackLink";
import { useTranslation } from "react-i18next";
import PageMeta from "../components/common/PageMeta";
import PageBreadcrumb from "../components/common/PageBreadCrumb";
import ComponentCard from "../components/common/ComponentCard";
import {
  Table,
  TableBody,
  TableCell,
  TableHeader,
  TableRow,
} from "../components/ui/table";
import Badge from "../components/ui/badge/Badge";
import IdentityFileThumbnail from "../components/persons/IdentityFileThumbnail";
import CaseTraceThumbnail from "../components/cases/CaseTraceThumbnail";
import { useCurrentUser } from "../context/CurrentUserContext";
import LinkBadge from "../components/analyses/LinkBadge";
import PersonArticle from "../components/articles/PersonArticle";
import { IdentityConflictList } from "../components/clusters/IdentityConflicts";
import RegisterFacts from "../components/persons/RegisterFacts";
import IntelligencePanels from "../components/common/IntelligencePanels";
import { extension } from "../extensions";
import type { IdentityConflict } from "../services/clusters";
import {
  getPersonIdByNumber,
  getPersonIdentity,
  personPath,
  getPersonProfile,
  listPersonCaseLinks,
  listPersonIdentityConflicts,
  type CaseCertainty,
  type PersonCaseLink,
  type PersonProfile,
  type PersonIdentityDocument,
  type PersonCluster,
  type PersonClusterMember,
} from "../services/persons";

function modalityLabelKey(modality: string): string {
  return modality === "FACIAL" ? "modality.facial" : "modality.fingerprint";
}

function modalityColor(modality: string): "info" | "warning" {
  return modality === "FACIAL" ? "info" : "warning";
}

function certaintyColor(certainty: CaseCertainty): "success" | "light" | "warning" | "error" {
  switch (certainty) {
    case "confirmed":
      return "success";
    case "uncertain":
      return "warning";
    case "excluded":
      return "error";
    default:
      return "light";
  }
}

function formatDate(iso: string, locale: string): string {
  return new Date(iso).toLocaleString(locale);
}

/** The best display name available -- the name on the first register of the
 * first document -- or the bare person ID when nothing has been enrolled
 * yet. */
function displayName(personId: string, documents: PersonIdentityDocument[]): string {
  for (const doc of documents) {
    for (const register of doc.registers) {
      if (register.name) return register.name;
    }
  }
  return personId;
}

/** One biometric sample inside a cluster -- an enrollment photo or a
 * crime-scene trace -- shown as a thumbnail tagged with its kind, so an
 * analyst can see at a glance which members are already-identified
 * enrollment records versus unresolved case evidence this cluster has
 * matched to them. */
function ClusterMemberCard({ member }: { member: PersonClusterMember }) {
  const { t } = useTranslation();
  if (member.kind === "KNOWN") {
    return (
      <div className="flex w-24 flex-col items-center gap-1.5 text-center">
        <IdentityFileThumbnail
          personId={member.personId ?? ""}
          identityFileId={member.identityFileId ?? 0}
          contentType={member.contentType}
          alt={member.name ?? member.personId ?? t("personProfile.enrolledRecord")}
        />
        <Badge size="sm" color="success">
          {t("personProfile.enrolled")}
        </Badge>
        <Link
          to={personPath(member.personNumber)}
          className="line-clamp-2 w-full break-words text-xs text-brand-500 hover:text-brand-600 dark:text-brand-400 dark:hover:text-brand-300"
        >
          {member.name}
        </Link>
      </div>
    );
  }

  return (
    <div className="flex w-24 flex-col items-center gap-1.5 text-center">
      <CaseTraceThumbnail
        caseId={member.caseId ?? ""}
        thumbnailFileId={member.thumbnailFileId}
        thumbnailBox={member.thumbnailBox}
        alt={t("personLookup.traceAlt", { traceId: member.traceId, caseId: member.caseId })}
      />
      <Badge size="sm" color="warning">
        {t("personProfile.caseEvidence")}
      </Badge>
      {member.link ? (
        <LinkBadge link={member.link} />
      ) : (
        <span className="text-[10px] text-gray-500 dark:text-gray-400">{t("links.indirect")}</span>
      )}
      <Link
        to={`/cases/${encodeURIComponent(member.caseId ?? "")}`}
        className="line-clamp-2 w-full break-all text-xs text-brand-500 hover:text-brand-600 dark:text-brand-400 dark:hover:text-brand-300"
      >
        {member.caseId}
      </Link>
    </div>
  );
}

/** One biometric cluster resolved to this person, with its full membership --
 * the replacement for a bare member-count row, so an analyst can see which
 * enrollment records and which case evidence a CONFIRMED decision chain has
 * actually grouped together, not just how many. */
function ClusterCard({ cluster }: { cluster: PersonCluster }) {
  const { t, i18n } = useTranslation();
  return (
    <div className="rounded-xl border border-gray-200 dark:border-white/[0.05]">
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-gray-100 px-5 py-3 dark:border-white/[0.05]">
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-medium text-gray-800 text-theme-sm dark:text-white/90">
            {t("personProfile.clusterNumber", { id: cluster.clusterId })}
          </span>
          <Badge size="sm" color={modalityColor(cluster.modality)}>
            {t(modalityLabelKey(cluster.modality))}
          </Badge>
          {cluster.hasCaseEvidence ? (
            <Badge size="sm" color="success">
              {t("personProfile.linkedToEvidence")}
            </Badge>
          ) : (
            <Badge size="sm" color="light">
              {t("personProfile.noEvidenceYet")}
            </Badge>
          )}
        </div>
        <span className="text-xs text-gray-500 dark:text-gray-400">
          {t("personProfile.clusterMeta", {
            count: cluster.memberCount,
            date: formatDate(cluster.createdAt, i18n.resolvedLanguage ?? "en"),
          })}
        </span>
      </div>
      <div className="flex flex-wrap gap-4 px-5 py-4">
        {cluster.members.map((member, index) => (
          <ClusterMemberCard
            key={
              member.kind === "KNOWN"
                ? `known-${member.identityFileId}`
                : `trace-${member.traceId}-${index}`
            }
            member={member}
          />
        ))}
      </div>
    </div>
  );
}

export default function PersonProfilePage() {
  const { t } = useTranslation();
  // The address carries the person's number (personPath); an older link may still have the person_id.
  const { personRef } = useParams<{ personRef: string }>();
  const navigate = useNavigate();
  const { isIntelligence } = useCurrentUser();
  const [profile, setProfile] = useState<PersonProfile | null>(null);
  // Every case, at every certainty (not only the reviewed profile.cases): intelligence only.
  const [caseLinks, setCaseLinks] = useState<PersonCaseLink[]>([]);
  const [conflicts, setConflicts] = useState<IdentityConflict[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  // Analysts see the article first (docs/person-article-plan.md §1); the data is a tab away.
  const [searchParams, setSearchParams] = useSearchParams();
  const tab = searchParams.get("tab") === "data" ? "data" : "article";
  const setTab = (next: "article" | "data") =>
    setSearchParams(next === "article" ? {} : { tab: next }, { replace: true });

  useEffect(() => {
    if (!personRef) return;
    let cancelled = false;

    async function load() {
      setLoading(true);
      setError("");
      try {
        if (!/^\d+$/.test(personRef!)) {
          // Swaps the person_id out of the address, and out of this history entry.
          const { personNumber } = await getPersonIdentity(personRef!);
          if (!cancelled) navigate({ pathname: personPath(personNumber), search: window.location.search }, { replace: true });
          return;
        }
        const personId = await getPersonIdByNumber(personRef!);
        // Anyone else may see who the person is, not where they have been.
        const [result, links, flagged] = isIntelligence
          ? await Promise.all([getPersonProfile(personId), listPersonCaseLinks(personId), listPersonIdentityConflicts(personId)])
          : [{ identity: await getPersonIdentity(personId), clusters: [], cases: [] }, [], []];
        if (!cancelled) {
          setProfile(result);
          setCaseLinks(links);
          setConflicts(flagged);
        }
      } catch (err) {
        if (!cancelled) {
          setError(
            err instanceof Error ? err.message : t("personProfile.loadError"),
          );
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    }

    load();
    return () => {
      cancelled = true;
    };
  }, [personRef, navigate, t, isIntelligence]);

  const documents = profile?.identity.documents ?? [];
  const clusters = profile?.clusters ?? [];
  const cases = caseLinks;

  return (
    <>
      <PageMeta
        title={`${t("personProfile.title")} | TrackID`}
        description={t("personProfile.metaDescription")}
      />
      <PageBreadcrumb pageTitle={t("personProfile.title")} />

      <div className="mb-6">
        <BackLink fallback="/persons" label={t("personProfile.backToLookup")} />
      </div>

      {loading && !profile && (
        <p className="text-sm text-gray-500 dark:text-gray-400">{t("common.loading")}</p>
      )}

      {!loading && error && <p className="text-sm text-error-500">{error}</p>}

      {profile && isIntelligence && conflicts.length > 0 && (
        <div className="mb-6">
          <ComponentCard title={t("identityConflict.title")} desc={t("identityConflict.desc")}>
            <IdentityConflictList conflicts={conflicts} />
          </ComponentCard>
        </div>
      )}

      {profile && isIntelligence && (
        <div className="mb-6 flex gap-1 border-b border-gray-200 dark:border-gray-800" role="tablist">
          {(["article", "data"] as const).map((key) => (
            <button
              key={key}
              type="button"
              role="tab"
              aria-selected={tab === key}
              onClick={() => setTab(key)}
              className={`-mb-px border-b-2 px-4 py-2 text-sm font-medium ${
                tab === key
                  ? "border-brand-500 text-brand-600 dark:text-brand-400"
                  : "border-transparent text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300"
              }`}
            >
              {t(`articles.tabs.${key}`)}
            </button>
          ))}
        </div>
      )}

      {profile && isIntelligence && tab === "article" && (
        <PersonArticle
          personId={profile.identity.personId}
          profile={profile}
          name={displayName(profile.identity.personId, documents)}
        />
      )}

      {profile && (!isIntelligence || tab === "data") && (
        <div className="space-y-6">
          <ComponentCard
            title={t("personProfile.person")}
            desc={t("personProfile.personId", { personId: profile.identity.personId })}
          >
            <span className="text-lg font-semibold text-gray-800 dark:text-white/90">
              {displayName(profile.identity.personId, documents)}
            </span>
          </ComponentCard>

          <IntelligencePanels panels={extension.personPanels} props={{ personId: profile.identity.personId }} />

          <ComponentCard
            title={t("personProfile.identity")}
            desc={t("personProfile.documentsOnFile", { count: documents.length })}
          >
            {documents.length === 0 && (
              <p className="text-sm text-gray-500 dark:text-gray-400">
                {t("personProfile.noDocuments")}
              </p>
            )}

            {documents.map((doc) => (
              <div
                key={doc.documentId}
                className="rounded-xl border border-gray-200 dark:border-white/[0.05]"
              >
                <div className="flex flex-wrap items-center justify-between gap-2 border-b border-gray-100 px-5 py-3 dark:border-white/[0.05]">
                  <div className="flex items-center gap-2">
                    <span className="font-medium text-gray-800 text-theme-sm dark:text-white/90">
                      {doc.documentType}
                    </span>
                    <span className="text-sm text-gray-500 dark:text-gray-400">
                      {doc.documentNumber}
                    </span>
                  </div>
                  {doc.fiscalNumber && (
                    <span className="text-xs text-gray-500 dark:text-gray-400">
                      {t("personProfile.fiscalNumber", { value: doc.fiscalNumber })}
                    </span>
                  )}
                </div>

                <div className="divide-y divide-gray-100 dark:divide-white/[0.05]">
                  {doc.registers.map((register) => (
                    <div key={register.registerId} className="px-5 py-4">
                      <div className="flex flex-wrap items-center justify-between gap-2">
                        <span className="font-medium text-gray-800 text-theme-sm dark:text-white/90">
                          {register.name}
                        </span>
                        <span className="text-xs text-gray-500 dark:text-gray-400">
                          {t("personProfile.register", { number: register.registerNumber })}
                        </span>
                      </div>
                      <div className="mt-3">
                        <RegisterFacts birthDate={register.birthDate} parent1Name={register.parent1Name} parent2Name={register.parent2Name} />
                      </div>

                      {register.files.length > 0 && (
                        <div className="mt-3 flex flex-wrap gap-3">
                          {register.files.map((file) => (
                            <div
                              key={file.identityFileId}
                              className="flex flex-col items-center gap-1.5"
                            >
                              <IdentityFileThumbnail
                                personId={profile.identity.personId}
                                identityFileId={file.identityFileId}
                                contentType={file.contentType}
                                alt={`${register.name} - ${file.fileType}`}
                              />
                              <Badge
                                size="sm"
                                color={file.featureId ? "success" : "light"}
                              >
                                {file.fileType}
                                {file.featureType ? ` · ${file.featureType}` : ""}
                              </Badge>
                            </div>
                          ))}
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              </div>
            ))}
          </ComponentCard>

          {/* Clusters and cases are a person's history: analysts only. */}
          {isIntelligence && (
                <>
              <ComponentCard
                title={t("clusters.title")}
                desc={t("personProfile.clustersResolved", { count: clusters.length })}
              >
                {clusters.length === 0 ? (
                  <div className="overflow-hidden rounded-xl border border-gray-200 bg-white px-5 py-8 text-center text-gray-500 dark:border-white/[0.05] dark:bg-white/[0.03] dark:text-gray-400">
                    {t("personProfile.noClusters")}
                  </div>
                ) : (
                  <div className="space-y-4">
                    {clusters.map((cluster) => (
                      <ClusterCard key={cluster.clusterId} cluster={cluster} />
                    ))}
                  </div>
                )}
              </ComponentCard>

              <ComponentCard
                title={t("personProfile.relatedCases")}
                desc={t("personProfile.casesAll", { count: cases.length })}
              >
                <div className="overflow-hidden rounded-xl border border-gray-200 bg-white dark:border-white/[0.05] dark:bg-white/[0.03]">
                  <div className="max-w-full overflow-x-auto">
                    <Table>
                      <TableHeader className="border-b border-gray-100 dark:border-white/[0.05]">
                        <TableRow>
                          <TableCell
                            isHeader
                            className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
                          >
                            {t("personProfile.columns.caseId")}
                          </TableCell>
                          <TableCell
                            isHeader
                            className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
                          >
                            {t("personProfile.columns.type")}
                          </TableCell>
                          <TableCell
                            isHeader
                            className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
                          >
                            {t("personProfile.columns.description")}
                          </TableCell>
                          <TableCell
                            isHeader
                            className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
                          >
                            {t("personProfile.columns.certainty")}
                          </TableCell>
                          <TableCell
                            isHeader
                            className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
                          >
                            {t("personProfile.columns.linkedVia")}
                          </TableCell>
                        </TableRow>
                      </TableHeader>
                      <TableBody className="divide-y divide-gray-100 dark:divide-white/[0.05]">
                        {cases.map((relatedCase) => (
                          <TableRow key={relatedCase.caseId}>
                            <TableCell className="px-5 py-4 sm:px-6 text-start">
                              <Link
                                to={`/cases/${encodeURIComponent(relatedCase.caseId)}`}
                                className="block font-medium text-brand-500 hover:text-brand-600 dark:text-brand-400 dark:hover:text-brand-300"
                              >
                                {relatedCase.caseId}
                              </Link>
                            </TableCell>
                            <TableCell className="px-4 py-3 text-start text-theme-sm">
                              <Badge
                                size="sm"
                                color={modalityColor(relatedCase.modality)}
                              >
                                {t(modalityLabelKey(relatedCase.modality))}
                              </Badge>
                            </TableCell>
                            <TableCell className="px-4 py-3 text-gray-500 text-start text-theme-sm dark:text-gray-400">
                              {relatedCase.description || "—"}
                            </TableCell>
                            <TableCell className="px-4 py-3 text-start text-theme-sm">
                              <Badge size="sm" color={certaintyColor(relatedCase.certainty)}>
                                {t(`articles.certainty.${relatedCase.certainty}`)}
                              </Badge>
                            </TableCell>
                            <TableCell className="px-4 py-3 text-start text-theme-sm">
                              <div className="flex flex-wrap items-center gap-1.5">
                                {relatedCase.reviewedClusterIds.map((clusterId) => (
                                  <Link key={clusterId} to={`/clusters/${clusterId}`}>
                                    <Badge size="sm" color="success">
                                      #{clusterId}
                                    </Badge>
                                  </Link>
                                ))}
                                {relatedCase.automaticClusterId && (
                                  <Link to={`/clusters/${relatedCase.automaticClusterId}`}>
                                    <Badge size="sm" color="light">
                                      #{relatedCase.automaticClusterId} · {t("personProfile.automaticCluster")}
                                    </Badge>
                                  </Link>
                                )}
                                {relatedCase.score !== undefined && (
                                  <span className="text-xs text-gray-500 dark:text-gray-400">
                                    {t("personProfile.score", { score: relatedCase.score.toFixed(2) })}
                                  </span>
                                )}
                              </div>
                            </TableCell>
                          </TableRow>
                        ))}

                        {cases.length === 0 && (
                          <TableRow>
                            <TableCell className="px-5 py-8 text-center text-gray-500 dark:text-gray-400">
                              {t("personProfile.noCases")}
                            </TableCell>
                          </TableRow>
                        )}
                      </TableBody>
                    </Table>
                  </div>
                </div>
              </ComponentCard>
            </>
          )}
        </div>
      )}
    </>
  );
}
