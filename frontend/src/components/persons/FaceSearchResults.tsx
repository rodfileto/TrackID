import { Link } from "react-router";
import { useTranslation } from "react-i18next";
import { Table, TableBody, TableCell, TableHeader, TableRow } from "../ui/table";
import Badge from "../ui/badge/Badge";
import IdentityFileThumbnail from "./IdentityFileThumbnail";
import CaseTraceThumbnail from "../cases/CaseTraceThumbnail";
import {
  personPath,
  type CaseFaceSearchResult,
  type ModalityCount,
  type PersonFaceSearchResult,
  type PersonSearchResult,
} from "../../services/persons";

// The result tables of a person search, by name or by face (the person lookup page, a case face's search).

function modalityLabelKey(modality: string): string {
  return modality === "FACIAL" ? "modality.facial" : "modality.fingerprint";
}

function modalityColor(modality: string): "info" | "warning" {
  return modality === "FACIAL" ? "info" : "warning";
}

/** The "N facial / N fingerprint" badges next to a person result -- how many
 * criminal cases (see PersonSearchResult.caseCounts) that person is linked
 * to, broken down by case type. Renders nothing when the person has none -- and for anyone but
 * an analyst, whom the server sends no counts. */
function CaseCountBadges({ caseCounts }: { caseCounts: ModalityCount[] | null }) {
  const { t } = useTranslation();
  if (!caseCounts || caseCounts.length === 0) {
    return <span className="text-gray-400 dark:text-white/30">—</span>;
  }
  return (
    <div className="flex flex-wrap gap-1">
      {caseCounts.map((c) => (
        <Badge key={c.modality} size="sm" color={modalityColor(c.modality)}>
          {c.count} {t(modalityLabelKey(c.modality))}
        </Badge>
      ))}
    </div>
  );
}

/** The columns every results table shares -- similarity and a match photo
 * are extra columns only the face search variant renders. */
export function ResultsTable({
  results,
  emptyMessage,
  showSimilarity,
}: {
  results: PersonSearchResult[] | PersonFaceSearchResult[];
  emptyMessage: string;
  showSimilarity: boolean;
}) {
  const { t } = useTranslation();
  return (
    <div className="overflow-hidden rounded-xl border border-gray-200 bg-white dark:border-white/[0.05] dark:bg-white/[0.03]">
      <div className="max-w-full overflow-x-auto">
        <Table>
          <TableHeader className="border-b border-gray-100 dark:border-white/[0.05]">
            <TableRow>
              {showSimilarity && (
                <TableCell
                  isHeader
                  className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
                >
                  {t("personLookup.columns.photo")}
                </TableCell>
              )}
              <TableCell
                isHeader
                className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
              >
                {t("personLookup.columns.name")}
              </TableCell>
              <TableCell
                isHeader
                className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
              >
                {t("personLookup.columns.document")}
              </TableCell>
              <TableCell
                isHeader
                className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
              >
                {t("personLookup.columns.register")}
              </TableCell>
              <TableCell
                isHeader
                className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
              >
                {t("personLookup.columns.cases")}
              </TableCell>
              {showSimilarity && (
                <TableCell
                  isHeader
                  className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
                >
                  {t("personLookup.columns.similarity")}
                </TableCell>
              )}
            </TableRow>
          </TableHeader>
          <TableBody className="divide-y divide-gray-100 dark:divide-white/[0.05]">
            {results.map((result, index) => (
              <TableRow key={`${result.personId}-${index}`}>
                {showSimilarity && (
                  <TableCell className="px-5 py-4 sm:px-6 text-start">
                    <IdentityFileThumbnail
                      personId={result.personId}
                      identityFileId={
                        (result as PersonFaceSearchResult).identityFileId
                      }
                      contentType={
                        (result as PersonFaceSearchResult).contentType
                      }
                      alt={result.name}
                    />
                  </TableCell>
                )}
                <TableCell className="px-5 py-4 sm:px-6 text-start">
                  <Link
                    to={personPath(result.personNumber)}
                    className="block font-medium text-brand-500 hover:text-brand-600 dark:text-brand-400 dark:hover:text-brand-300"
                  >
                    {result.name}
                  </Link>
                </TableCell>
                <TableCell className="px-4 py-3 text-gray-500 text-start text-theme-sm dark:text-gray-400">
                  {result.documentType} {result.documentNumber}
                </TableCell>
                <TableCell className="px-4 py-3 text-gray-500 text-start text-theme-sm dark:text-gray-400">
                  {result.registerNumber}
                </TableCell>
                <TableCell className="px-4 py-3 text-start text-theme-sm">
                  <CaseCountBadges caseCounts={result.caseCounts} />
                </TableCell>
                {showSimilarity && (
                  <TableCell className="px-4 py-3 text-gray-500 text-start text-theme-sm dark:text-gray-400">
                    {((result as PersonFaceSearchResult).similarity * 100).toFixed(1)}%
                  </TableCell>
                )}
              </TableRow>
            ))}

            {results.length === 0 && (
              <TableRow>
                <TableCell
                  colSpan={showSimilarity ? 6 : 4}
                  className="px-5 py-8 text-center text-gray-500 dark:text-gray-400"
                >
                  {emptyMessage}
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </div>
    </div>
  );
}

/** The "Matching criminal cases" table a face search's QUESTIONED matches
 * render into, below the enrolled-person results -- similarity here isn't
 * comparable to a person result's (KNOWN vs QUESTIONED embeddings), so this
 * stays a separate table rather than a merged ranked list. */
export function CaseResultsTable({ results }: { results: CaseFaceSearchResult[] }) {
  const { t } = useTranslation();
  return (
    <div className="overflow-hidden rounded-xl border border-gray-200 bg-white dark:border-white/[0.05] dark:bg-white/[0.03]">
      <div className="max-w-full overflow-x-auto">
        <Table>
          <TableHeader className="border-b border-gray-100 dark:border-white/[0.05]">
            <TableRow>
              <TableCell
                isHeader
                className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
              >
                {t("personLookup.columns.photo")}
              </TableCell>
              <TableCell
                isHeader
                className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
              >
                {t("personLookup.columns.caseId")}
              </TableCell>
              <TableCell
                isHeader
                className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
              >
                {t("personLookup.columns.type")}
              </TableCell>
              <TableCell
                isHeader
                className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
              >
                {t("personLookup.columns.description")}
              </TableCell>
              <TableCell
                isHeader
                className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
              >
                {t("personLookup.columns.similarity")}
              </TableCell>
            </TableRow>
          </TableHeader>
          <TableBody className="divide-y divide-gray-100 dark:divide-white/[0.05]">
            {results.map((result) => (
              <TableRow key={`${result.caseId}-${result.traceId}`}>
                <TableCell className="px-5 py-4 sm:px-6 text-start">
                  <CaseTraceThumbnail
                    caseId={result.caseId}
                    thumbnailFileId={result.thumbnailFileId}
                    thumbnailBox={result.thumbnailBox}
                    alt={t("personLookup.traceAlt", { traceId: result.traceId, caseId: result.caseId })}
                  />
                </TableCell>
                <TableCell className="px-5 py-4 sm:px-6 text-start">
                  <Link
                    to={`/cases/${encodeURIComponent(result.caseId)}`}
                    className="block font-medium text-brand-500 hover:text-brand-600 dark:text-brand-400 dark:hover:text-brand-300"
                  >
                    {result.caseId}
                  </Link>
                </TableCell>
                <TableCell className="px-4 py-3 text-start text-theme-sm">
                  <Badge size="sm" color={modalityColor(result.modality)}>
                    {t(modalityLabelKey(result.modality))}
                  </Badge>
                </TableCell>
                <TableCell className="px-4 py-3 text-gray-500 text-start text-theme-sm dark:text-gray-400">
                  {result.description || "—"}
                </TableCell>
                <TableCell className="px-4 py-3 text-gray-500 text-start text-theme-sm dark:text-gray-400">
                  {(result.similarity * 100).toFixed(1)}%
                </TableCell>
              </TableRow>
            ))}

            {results.length === 0 && (
              <TableRow>
                <TableCell
                  colSpan={5}
                  className="px-5 py-8 text-center text-gray-500 dark:text-gray-400"
                >
                  {t("personLookup.noCasesForFace")}
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </div>
    </div>
  );
}
