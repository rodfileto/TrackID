import type { ReactNode } from "react";
import { useDropzone, type Accept, type FileRejection } from "react-dropzone";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

interface FileDropzoneProps {
  /** Called with the accepted files, whether dropped or picked. */
  onFiles: (files: File[]) => void;
  /** What may be added; see fileTypes.ts. Anything, when left out. */
  accept?: Accept;
  multiple?: boolean;
  disabled?: boolean;
  /** "panel" for an open area, "compact" for a narrow column, "inline" for a toolbar. */
  variant?: "panel" | "compact" | "inline";
  /** Replaces the default text: the heading of a panel, the whole of an inline zone. */
  label?: ReactNode;
  /** A line under the heading of a panel. */
  description?: ReactNode;
  /** Replaces the default message when a file is refused. */
  onRejected?: () => void;
  /** The input's id, for a <Label htmlFor>. */
  id?: string;
  className?: string;
}

function UploadIcon({ size }: { size: number }) {
  return (
    <svg className="fill-current" width={size} height={size} viewBox="0 0 29 28" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
      <path
        fillRule="evenodd"
        clipRule="evenodd"
        d="M14.5019 3.91699C14.2852 3.91699 14.0899 4.00891 13.953 4.15589L8.57363 9.53186C8.28065 9.82466 8.2805 10.2995 8.5733 10.5925C8.8661 10.8855 9.34097 10.8857 9.63396 10.5929L13.7519 6.47752V18.667C13.7519 19.0812 14.0877 19.417 14.5019 19.417C14.9161 19.417 15.2519 19.0812 15.2519 18.667V6.48234L19.3653 10.5929C19.6583 10.8857 20.1332 10.8855 20.426 10.5925C20.7188 10.2995 20.7186 9.82463 20.4256 9.53184L15.0838 4.19378C14.9463 4.02488 14.7367 3.91699 14.5019 3.91699ZM5.91626 18.667C5.91626 18.2528 5.58047 17.917 5.16626 17.917C4.75205 17.917 4.41626 18.2528 4.41626 18.667V21.8337C4.41626 23.0763 5.42362 24.0837 6.66626 24.0837H22.3339C23.5766 24.0837 24.5839 23.0763 24.5839 21.8337V18.667C24.5839 18.2528 24.2482 17.917 23.8339 17.917C23.4197 17.917 23.0839 18.2528 23.0839 18.667V21.8337C23.0839 22.2479 22.7482 22.5837 22.3339 22.5837H6.66626C6.25205 22.5837 5.91626 22.2479 5.91626 21.8337V18.667Z"
      />
    </svg>
  );
}

/** The app's one way to add files: drop them here, or click (or press Enter) to pick them. */
export default function FileDropzone({
  onFiles,
  accept,
  multiple = false,
  disabled = false,
  variant = "panel",
  label,
  description,
  onRejected,
  id,
  className = "",
}: FileDropzoneProps) {
  const { t } = useTranslation();

  function handleDrop(accepted: File[], rejections: FileRejection[]) {
    if (accepted.length > 0) onFiles(accepted);
    if (rejections.length === 0) return;
    if (onRejected) onRejected();
    else if (rejections.some((r) => r.errors.some((e) => e.code === "too-many-files"))) toast.error(t("dropzone.tooMany"));
    else toast.error(t("dropzone.unsupported"));
  }

  const { getRootProps, getInputProps, isDragActive, isDragReject } = useDropzone({
    onDrop: handleDrop,
    accept,
    multiple,
    disabled,
  });

  const state = disabled
    ? "cursor-not-allowed border-gray-200 bg-gray-50 opacity-60 dark:border-gray-800 dark:bg-gray-900"
    : isDragReject
      ? "cursor-copy border-error-500 bg-error-50 dark:bg-error-500/10"
      : isDragActive
        ? "cursor-copy border-brand-500 bg-brand-50 dark:bg-brand-500/10"
        : "cursor-pointer border-gray-300 bg-gray-50 hover:border-brand-500 dark:border-gray-700 dark:bg-gray-900 dark:hover:border-brand-500";
  const focus = "focus:outline-hidden focus-visible:ring-3 focus-visible:ring-brand-500/30";
  const root = getRootProps({ role: "button", "aria-disabled": disabled });
  const input = <input {...getInputProps({ id })} />;

  const dragHint = multiple ? t("dropzone.dragHere") : t("dropzone.dragHereOne");
  const dragText = isDragReject
    ? t("dropzone.unsupported")
    : multiple
      ? t("dropzone.dropHere")
      : t("dropzone.dropHereOne");

  if (variant === "inline") {
    return (
      <div
        {...root}
        title={dragHint}
        className={`inline-flex min-h-10 max-w-full items-center gap-2 rounded-lg border border-dashed px-3 py-1.5 text-sm font-medium text-gray-700 transition dark:text-gray-300 ${state} ${focus} ${className}`}
      >
        {input}
        <span className="shrink-0 text-brand-500">
          <UploadIcon size={18} />
        </span>
        <span className="truncate">{isDragActive ? dragText : (label ?? dragHint)}</span>
      </div>
    );
  }

  const compact = variant === "compact";
  // A panel with its own heading still says that files can be dropped on it.
  const detail = description ?? (label ? dragHint : null);
  return (
    <div
      {...root}
      className={`flex flex-col items-center justify-center rounded-xl border border-dashed text-center transition ${
        compact ? "p-4" : "p-7 lg:p-10"
      } ${state} ${focus} ${className}`}
    >
      {input}
      <div className={`flex justify-center ${compact ? "mb-3" : "mb-[22px]"}`}>
        <div
          className={`flex items-center justify-center rounded-full bg-gray-200 text-gray-700 dark:bg-gray-800 dark:text-gray-400 ${
            compact ? "h-11 w-11" : "h-[68px] w-[68px]"
          }`}
        >
          <UploadIcon size={28} />
        </div>
      </div>
      <h4 className={`max-w-full truncate font-semibold text-gray-800 dark:text-white/90 ${compact ? "mb-2 text-sm" : "mb-3 text-theme-xl"}`}>
        {isDragActive ? dragText : (label ?? dragHint)}
      </h4>
      {detail && (
        <span className={`block w-full max-w-[420px] text-gray-700 dark:text-gray-400 ${compact ? "mb-2 text-xs" : "mb-5 text-sm"}`}>
          {detail}
        </span>
      )}
      <span className="font-medium underline text-theme-sm text-brand-500">
        {multiple ? t("dropzone.browse") : t("dropzone.browseOne")}
      </span>
    </div>
  );
}
