import { useEffect, useState } from "react";
import { Link, useParams } from "react-router";
import { toast } from "sonner";
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
import {
  addEvidence,
  getCase,
  type CaseDetail,
  type Evidence,
} from "../services/cases";

function caseTypeLabel(caseType: string): string {
  return caseType === "FACIAL" ? "Facial" : "Fingerprint";
}

function caseTypeColor(caseType: string): "info" | "warning" {
  return caseType === "FACIAL" ? "info" : "warning";
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

function formatDate(iso: string): string {
  return new Date(iso).toLocaleString();
}

export default function CaseDetailPage() {
  const { caseId } = useParams<{ caseId: string }>();
  const [detail, setDetail] = useState<CaseDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const [selectedFiles, setSelectedFiles] = useState<File[]>([]);
  const [uploading, setUploading] = useState(false);

  async function load() {
    if (!caseId) return;
    setLoading(true);
    setError("");
    try {
      setDetail(await getCase(caseId));
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not load case");
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
    toast.error(
      "Unsupported file type. Allowed: pdf, jpg, png, webp, gif, tiff, bmp.",
    );
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
      toast.success(
        `${added} evidence file${added === 1 ? "" : "s"} added.`,
      );
    } else if (added > 0) {
      toast.warning(`${added} added, ${failed} failed.`);
    } else {
      toast.error("No evidence files were uploaded.");
    }

    setSelectedFiles([]);
    setUploading(false);
    await load();
  }

  const evidences: Evidence[] = detail?.evidences ?? [];

  return (
    <>
      <PageMeta
        title={`Case ${caseId ?? ""} | TrackID`}
        description="Case detail"
      />
      <PageBreadcrumb pageTitle={`Case ${caseId ?? ""}`} />

      <div className="mb-6">
        <Link
          to="/cases"
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
          Back to cases
        </Link>
      </div>

      {loading && (
        <p className="text-sm text-gray-500 dark:text-gray-400">Loading...</p>
      )}

      {!loading && error && (
        <p className="text-sm text-error-500">{error}</p>
      )}

      {!loading && detail && (
        <div className="space-y-6">
          <ComponentCard
            title="Case"
            desc={detail.description || "No description."}
          >
            <div className="flex items-center gap-4">
              <span className="font-medium text-gray-800 text-theme-sm dark:text-white/90">
                {detail.caseId}
              </span>
              <Badge size="sm" color={caseTypeColor(detail.caseType)}>
                {caseTypeLabel(detail.caseType)}
              </Badge>
            </div>
          </ComponentCard>

          <ComponentCard title="Add Evidence" desc="Attach digital files.">
            <EvidenceDropzone onFiles={handleFiles} onRejected={handleRejected} />

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
                      Remove
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
                  ? "Uploading..."
                  : selectedFiles.length > 1
                    ? `Upload ${selectedFiles.length} files`
                    : "Upload"}
              </Button>
            </div>
          </ComponentCard>

          <ComponentCard
            title="Evidences"
            desc={`${evidences.length} file${evidences.length === 1 ? "" : "s"}`}
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
                        Preview
                      </TableCell>
                      <TableCell
                        isHeader
                        className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
                      >
                        File
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
                        Size
                      </TableCell>
                      <TableCell
                        isHeader
                        className="px-5 py-3 font-medium text-gray-500 text-start text-theme-xs dark:text-gray-400"
                      >
                        Uploaded
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
                          {formatDate(evidence.createdAt)}
                        </TableCell>
                      </TableRow>
                    ))}

                    {evidences.length === 0 && (
                      <TableRow>
                        <TableCell className="px-5 py-8 text-center text-gray-500 dark:text-gray-400">
                          No evidence yet.
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
