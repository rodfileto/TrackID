import { useEffect, useState } from "react";
import { Link, useParams } from "react-router";
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
import {
  getPersonProfile,
  type PersonProfile,
  type PersonIdentityDocument,
} from "../services/persons";

function caseTypeLabel(caseType: string): string {
  return caseType === "FACIAL" ? "Facial" : "Fingerprint";
}

function caseTypeColor(caseType: string): "info" | "warning" {
  return caseType === "FACIAL" ? "info" : "warning";
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleString();
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

export default function PersonProfilePage() {
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
            err instanceof Error ? err.message : "Could not load person",
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
  }, [personId]);

  const documents = profile?.identity.documents ?? [];
  const clusters = profile?.clusters ?? [];
  const cases = profile?.cases ?? [];

  return (
    <>
      <PageMeta
        title={`Person ${personId ?? ""} | TrackID`}
        description="Person profile"
      />
      <PageBreadcrumb pageTitle="Person Profile" />

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
          Back to lookup
        </Link>
      </div>

      {loading && !profile && (
        <p className="text-sm text-gray-500 dark:text-gray-400">Loading...</p>
      )}

      {!loading && error && <p className="text-sm text-error-500">{error}</p>}

      {profile && (
        <div className="space-y-6">
          <ComponentCard
            title="Person"
            desc={`Person ID ${profile.identity.personId}`}
          >
            <span className="text-lg font-semibold text-gray-800 dark:text-white/90">
              {displayName(profile.identity.personId, documents)}
            </span>
          </ComponentCard>

          <ComponentCard
            title="Identity"
            desc={`${documents.length} document${documents.length === 1 ? "" : "s"} on file`}
          >
            {documents.length === 0 && (
              <p className="text-sm text-gray-500 dark:text-gray-400">
                No identity documents enrolled for this person.
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
                      Fiscal number: {doc.fiscalNumber}
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
                          Register {register.registerNumber}
                          {register.birthDate ? ` · b. ${register.birthDate}` : ""}
                        </span>
                      </div>
                      <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
                        Parents: {register.parent1Name} &amp; {register.parent2Name}
                      </p>

                      {register.files.length > 0 && (
                        <div className="mt-3 flex flex-wrap gap-2">
                          {register.files.map((file) => (
                            <Badge
                              key={file.identityFileId}
                              size="sm"
                              color={file.featureId ? "success" : "light"}
                            >
                              {file.fileType}
                              {file.featureType ? ` · ${file.featureType}` : ""}
                            </Badge>
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
            title="Biometric Clusters"
            desc={`${clusters.length} cluster${clusters.length === 1 ? "" : "s"} resolved to this person`}
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
                        Cluster
                      </TableCell>
                      <TableCell
                        isHeader
                        className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
                      >
                        Modality
                      </TableCell>
                      <TableCell
                        isHeader
                        className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
                      >
                        Members
                      </TableCell>
                      <TableCell
                        isHeader
                        className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
                      >
                        Created
                      </TableCell>
                    </TableRow>
                  </TableHeader>
                  <TableBody className="divide-y divide-gray-100 dark:divide-white/[0.05]">
                    {clusters.map((cluster) => (
                      <TableRow key={cluster.clusterId}>
                        <TableCell className="px-5 py-4 sm:px-6 text-start">
                          <span className="font-medium text-gray-800 text-theme-sm dark:text-white/90">
                            #{cluster.clusterId}
                          </span>
                        </TableCell>
                        <TableCell className="px-4 py-3 text-start text-theme-sm">
                          <Badge size="sm" color={caseTypeColor(cluster.caseType)}>
                            {caseTypeLabel(cluster.caseType)}
                          </Badge>
                        </TableCell>
                        <TableCell className="px-4 py-3 text-gray-500 text-start text-theme-sm dark:text-gray-400">
                          {cluster.memberCount}
                        </TableCell>
                        <TableCell className="px-4 py-3 text-gray-500 text-start text-theme-sm dark:text-gray-400">
                          {formatDate(cluster.createdAt)}
                        </TableCell>
                      </TableRow>
                    ))}

                    {clusters.length === 0 && (
                      <TableRow>
                        <TableCell className="px-5 py-8 text-center text-gray-500 dark:text-gray-400">
                          Not yet linked to any biometric cluster.
                        </TableCell>
                      </TableRow>
                    )}
                  </TableBody>
                </Table>
              </div>
            </div>
          </ComponentCard>

          <ComponentCard
            title="Related Cases"
            desc={`${cases.length} criminal case${cases.length === 1 ? "" : "s"} linked through resolved clusters`}
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
                        Case ID
                      </TableCell>
                      <TableCell
                        isHeader
                        className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
                      >
                        Type
                      </TableCell>
                      <TableCell
                        isHeader
                        className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
                      >
                        Description
                      </TableCell>
                      <TableCell
                        isHeader
                        className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
                      >
                        Linked via
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
                            {caseTypeLabel(relatedCase.caseType)}
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
                          No criminal cases linked yet.
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
