import { useEffect, useState } from "react";
import { Link, useLocation, useParams } from "react-router";
import { toast } from "sonner";
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
import Button from "../components/ui/button/Button";
import EvidenceDropzone from "../components/cases/EvidenceDropzone";
import EvidencePreview from "../components/cases/EvidencePreview";
import FacialCaseWorkspace from "../components/cases/FacialCaseWorkspace";
import TraceMarkerModal from "../components/cases/TraceMarkerModal";
import {
  addEvidence,
  deleteEvidence,
  getCase,
  type CaseDetail,
  type Evidence,
} from "../services/cases";

function caseTypeLabelKey(caseType: string): string {
  return caseType === "CIVIL" ? "caseType.civil" : "caseType.criminal";
}

function caseTypeColor(caseType: string): "error" | "success" {
  return caseType === "CIVIL" ? "success" : "error";
}

function modalityLabelKey(modality: string): string {
  return modality === "FACIAL" ? "modality.facial" : "modality.fingerprint";
}

function modalityColor(modality: string): "info" | "warning" {
  return modality === "FACIAL" ? "info" : "warning";
}

function mediaTypeColor(mediaType: string): "info" | "warning" | "light" {
  if (mediaType === "image") return "info";
  if (mediaType === "pdf") return "warning";
  return "light";
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function formatDate(iso: string, locale: string): string {
  return new Date(iso).toLocaleString(locale);
}

export default function CaseDetailPage() {
  const { t, i18n } = useTranslation();
  const { caseId } = useParams<{ caseId: string }>();
  const location = useLocation();
  const backToCases =
    (location.state as { from?: string } | null)?.from ?? "/cases";
  const [detail, setDetail] = useState<CaseDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const [selectedFiles, setSelectedFiles] = useState<File[]>([]);
  const [uploading, setUploading] = useState(false);

  const [traceEvidence, setTraceEvidence] = useState<Evidence | null>(null);
  const [excludingId, setExcludingId] = useState<number | null>(null);

  async function load() {
    if (!caseId) return;
    setLoading(true);
    setError("");
    try {
      setDetail(await getCase(caseId));
    } catch (err) {
      setError(err instanceof Error ? err.message : t("caseDetail.loadError"));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [caseId]);

  function handleFiles(files: File[]) {
    setSelectedFiles((current) => [...current, ...files]);
  }

  function handleRejected() {
    toast.error(t("caseDetail.unsupportedFile"));
  }

  function removeFile(index: number) {
    setSelectedFiles((current) => current.filter((_, i) => i !== index));
  }

  async function handleUpload() {
    if (!caseId || selectedFiles.length === 0) return;
    setUploading(true);

    let added = 0;
    let failed = 0;
    for (const file of selectedFiles) {
      try {
        await addEvidence(caseId, file);
        added++;
      } catch {
        failed++;
      }
    }

    if (failed === 0) {
      toast.success(t("caseDetail.evidenceAdded", { count: added }));
    } else if (added > 0) {
      toast.warning(t("caseDetail.partialUpload", { added, failed }));
    } else {
      toast.error(t("caseDetail.noneUploaded"));
    }

    setSelectedFiles([]);
    setUploading(false);
    await load();
  }

  async function handleExcludeEvidence(evidence: Evidence) {
    if (!caseId) return;
    if (
      !window.confirm(
        t("caseDetail.confirmExclude", { filename: evidence.filename }),
      )
    ) {
      return;
    }
    setExcludingId(evidence.id);
    try {
      await deleteEvidence(caseId, evidence.id);
      toast.success(t("caseDetail.evidenceExcluded"));
      await load();
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : t("caseDetail.excludeError"),
      );
    } finally {
      setExcludingId(null);
    }
  }

  const evidences: Evidence[] = detail?.evidences ?? [];

  return (
    <>
      <PageMeta
        title={`${t("caseDetail.title", { caseId: caseId ?? "" })} | TrackID`}
        description={t("caseDetail.metaDescription")}
      />
      <PageBreadcrumb pageTitle={t("caseDetail.title", { caseId: caseId ?? "" })} />

      <div className="mb-6">
        <Link
          to={backToCases}
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
          {t("caseDetail.backToCases")}
        </Link>
      </div>

      {/* Only the first load blanks the page: reloads after an upload or
          exclude keep the workspace mounted, with its unsaved boxes. */}
      {loading && !detail && (
        <p className="text-sm text-gray-500 dark:text-gray-400">{t("common.loading")}</p>
      )}

      {!loading && error && (
        <p className="text-sm text-error-500">{error}</p>
      )}

      {detail && (
        <div className="space-y-6">
          <ComponentCard
            title={t("caseDetail.case")}
            desc={detail.description || t("caseDetail.noDescription")}
          >
            <div className="flex items-center gap-4">
              <span className="font-medium text-gray-800 text-theme-sm dark:text-white/90">
                {detail.caseId}
              </span>
              <Badge size="sm" color={caseTypeColor(detail.caseType)}>
                {t(caseTypeLabelKey(detail.caseType))}
              </Badge>
              <Badge size="sm" color={modalityColor(detail.modality)}>
                {t(modalityLabelKey(detail.modality))}
              </Badge>
            </div>
          </ComponentCard>

          {detail.modality === "FACIAL" ? (
            <FacialCaseWorkspace
              caseId={caseId ?? ""}
              evidences={evidences}
              onEvidencesChanged={load}
            />
          ) : (
          <div className="grid grid-cols-1 gap-6 lg:grid-cols-12">
            <div className="lg:col-span-4">
              <ComponentCard
                title={t("caseDetail.addEvidence")}
                desc={t("caseDetail.attachFiles")}
                className="h-full"
              >
                <EvidenceDropzone
                  onFiles={handleFiles}
                  onRejected={handleRejected}
                />

                {selectedFiles.length > 0 && (
                  <ul className="mt-4 divide-y divide-gray-100 dark:divide-white/[0.05]">
                    {selectedFiles.map((file, index) => (
                      <li
                        key={`${file.name}-${index}`}
                        className="flex items-center justify-between py-2"
                      >
                        <div className="flex items-center gap-3">
                          <span className="text-sm font-medium text-gray-800 dark:text-white/90">
                            {file.name}
                          </span>
                          <span className="text-xs text-gray-500 dark:text-gray-400">
                            {formatBytes(file.size)}
                          </span>
                        </div>
                        <button
                          type="button"
                          onClick={() => removeFile(index)}
                          className="text-sm text-error-500 hover:text-error-600 dark:text-error-400"
                        >
                          {t("common.remove")}
                        </button>
                      </li>
                    ))}
                  </ul>
                )}

                <div className="mt-4 flex justify-end">
                  <Button
                    size="sm"
                    onClick={handleUpload}
                    disabled={selectedFiles.length === 0 || uploading}
                  >
                    {uploading
                      ? t("caseDetail.uploading")
                      : selectedFiles.length > 1
                        ? t("caseDetail.uploadN", { count: selectedFiles.length })
                        : t("caseDetail.upload")}
                  </Button>
                </div>
              </ComponentCard>
            </div>

            <div className="lg:col-span-8">
              <ComponentCard
                title={t("caseDetail.evidences")}
                desc={t("caseDetail.fileCount", { count: evidences.length })}
                className="h-full"
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
                        {t("caseDetail.columns.preview")}
                      </TableCell>
                      <TableCell
                        isHeader
                        className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
                      >
                        {t("caseDetail.columns.file")}
                      </TableCell>
                      <TableCell
                        isHeader
                        className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
                      >
                        {t("caseDetail.columns.type")}
                      </TableCell>
                      <TableCell
                        isHeader
                        className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
                      >
                        {t("caseDetail.columns.size")}
                      </TableCell>
                      <TableCell
                        isHeader
                        className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
                      >
                        {t("caseDetail.columns.uploaded")}
                      </TableCell>
                      <TableCell
                        isHeader
                        className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
                      >
                        {t("caseDetail.columns.actions")}
                      </TableCell>
                    </TableRow>
                  </TableHeader>

                  <TableBody className="divide-y divide-gray-100 dark:divide-white/[0.05]">
                    {evidences.map((evidence) => (
                      <TableRow key={evidence.id}>
                        <TableCell className="px-5 py-4 sm:px-6 text-start">
                          <EvidencePreview
                            caseId={caseId ?? ""}
                            evidenceId={evidence.id}
                            filename={evidence.filename}
                            mediaType={evidence.mediaType}
                            contentType={evidence.contentType}
                          />
                        </TableCell>
                        <TableCell className="px-5 py-4 sm:px-6 text-start">
                          <span className="block font-medium text-gray-800 text-theme-sm dark:text-white/90">
                            {evidence.filename}
                          </span>
                        </TableCell>
                        <TableCell className="px-4 py-3 text-start text-theme-sm">
                          <Badge
                            size="sm"
                            color={mediaTypeColor(evidence.mediaType ?? "")}
                          >
                            {evidence.mediaType ?? "file"}
                          </Badge>
                        </TableCell>
                        <TableCell className="px-4 py-3 text-gray-500 text-start text-theme-sm dark:text-gray-400">
                          {formatBytes(evidence.sizeBytes)}
                        </TableCell>
                        <TableCell className="px-4 py-3 text-gray-500 text-start text-theme-sm dark:text-gray-400">
                          {formatDate(evidence.createdAt, i18n.resolvedLanguage ?? "en")}
                        </TableCell>
                        <TableCell className="px-4 py-3 text-start text-theme-sm">
                          <div className="flex items-center gap-2">
                            {evidence.contentType?.startsWith("image/") && (
                              <button
                                type="button"
                                onClick={() => setTraceEvidence(evidence)}
                                className="inline-flex h-8 items-center rounded-lg border border-gray-200 bg-white px-3 text-xs font-medium text-gray-700 shadow-theme-xs hover:bg-gray-50 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-300 dark:hover:bg-white/[0.03]"
                              >
                                {t("caseDetail.markTraces")}
                              </button>
                            )}
                            <button
                              type="button"
                              onClick={() => handleExcludeEvidence(evidence)}
                              disabled={excludingId === evidence.id}
                              className="inline-flex h-8 items-center rounded-lg border border-gray-200 bg-white px-3 text-xs font-medium text-error-500 shadow-theme-xs hover:bg-error-50 disabled:opacity-50 dark:border-gray-700 dark:bg-gray-900 dark:text-error-400 dark:hover:bg-white/[0.03]"
                            >
                              {excludingId === evidence.id
                                ? t("caseDetail.excluding")
                                : t("caseDetail.exclude")}
                            </button>
                          </div>
                        </TableCell>
                      </TableRow>
                    ))}

                    {evidences.length === 0 && (
                      <TableRow>
                        <TableCell className="px-5 py-8 text-center text-gray-500 dark:text-gray-400">
                          {t("caseDetail.noEvidence")}
                        </TableCell>
                      </TableRow>
                    )}
                  </TableBody>
                    </Table>
                  </div>
                </div>
              </ComponentCard>
            </div>
          </div>
          )}
        </div>
      )}

      {traceEvidence && (
        <TraceMarkerModal
          caseId={caseId ?? ""}
          modality={detail?.modality ?? ""}
          evidenceId={traceEvidence.id}
          filename={traceEvidence.filename}
          isOpen={!!traceEvidence}
          onClose={() => setTraceEvidence(null)}
        />
      )}
    </>
  );
}
