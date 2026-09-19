import { useEffect, useState } from "react";
import { Link, useParams } from "react-router";
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
import {
  getPersonProfile,
  type PersonProfile,
  type PersonIdentityDocument,
  type PersonCluster,
  type PersonClusterMember,
} from "../services/persons";

function caseTypeLabelKey(caseType: string): string {
  return caseType === "FACIAL" ? "caseType.facial" : "caseType.fingerprint";
}

function caseTypeColor(caseType: string): "info" | "warning" {
  return caseType === "FACIAL" ? "info" : "warning";
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
          to={`/persons/${encodeURIComponent(member.personId ?? "")}`}
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
          <Badge size="sm" color={caseTypeColor(cluster.caseType)}>
            {t(caseTypeLabelKey(cluster.caseType))}
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
  const { personId } = useParams<{ personId: string }>();
  const [profile, setProfile] = useState<PersonProfile | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!personId) return;
    let cancelled = false;

    async function load() {
      setLoading(true);
      setError("");
      try {
        const result = await getPersonProfile(personId!);
        if (!cancelled) setProfile(result);
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
  }, [personId, t]);

  const documents = profile?.identity.documents ?? [];
  const clusters = profile?.clusters ?? [];
  const cases = profile?.cases ?? [];

  return (
    <>
      <PageMeta
        title={`${t("personProfile.personTitle", { personId: personId ?? "" })} | TrackID`}
        description={t("personProfile.metaDescription")}
      />
      <PageBreadcrumb pageTitle={t("personProfile.title")} />

      <div className="mb-6">
        <Link
          to="/persons"
          className="inline-flex items-center gap-1.5 text-sm text-gray-500 transition-colors hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300"
        >
          <svg
            className="stroke-current"
            width="17"
            height="16"
            viewBox="0 0 17 16"
            fill="none"
            xmlns="http://www.w3.org/2000/svg"
          >
            <path
              d="M10.9235 12.667L6.75683 8.50033L10.9235 4.33366"
              stroke=""
              strokeWidth="1.2"
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          </svg>
          {t("personProfile.backToLookup")}
        </Link>
      </div>

      {loading && !profile && (
        <p className="text-sm text-gray-500 dark:text-gray-400">{t("common.loading")}</p>
      )}

      {!loading && error && <p className="text-sm text-error-500">{error}</p>}

      {profile && (
        <div className="space-y-6">
          <ComponentCard
            title={t("personProfile.person")}
            desc={t("personProfile.personId", { personId: profile.identity.personId })}
          >
            <span className="text-lg font-semibold text-gray-800 dark:text-white/90">
              {displayName(profile.identity.personId, documents)}
            </span>
          </ComponentCard>

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
                          {register.birthDate
                            ? ` · ${t("personProfile.born", { date: register.birthDate })}`
                            : ""}
                        </span>
                      </div>
                      <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
                        {t("personProfile.parents", { parent1: register.parent1Name, parent2: register.parent2Name })}
                      </p>

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
            desc={t("personProfile.casesLinked", { count: cases.length })}
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
                            color={caseTypeColor(relatedCase.caseType)}
                          >
                            {t(caseTypeLabelKey(relatedCase.caseType))}
                          </Badge>
                        </TableCell>
                        <TableCell className="px-4 py-3 text-gray-500 text-start text-theme-sm dark:text-gray-400">
                          {relatedCase.description || "—"}
                        </TableCell>
                        <TableCell className="px-4 py-3 text-start text-theme-sm">
                          <div className="flex flex-wrap gap-1.5">
                            {relatedCase.clusterIds.map((clusterId) => (
                              <Badge key={clusterId} size="sm" color="light">
                                #{clusterId}
                              </Badge>
                            ))}
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
        </div>
      )}
    </>
  );
}
