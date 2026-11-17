import { useEffect, useState } from "react";

interface Props {
  file: File;
  /** Side of the square box, in pixels. */
  size?: number;
  className?: string;
}

// The longer side the PDF's first page is rendered at: enough for a sharp thumbnail on a dense
// screen, little enough to render in a moment.
const PDF_RENDER_SIDE = 320;

// A small picture of a file the operator just added: the image itself, or the first page of a PDF.
// Anything else, or a PDF that cannot be rendered, shows its extension.
export default function FileThumbnail({ file, size = 56, className = "" }: Props) {
  const [src, setSrc] = useState<string | null>(null);
  const isImage = file.type.startsWith("image/");
  const isPdf = file.type === "application/pdf" || file.name.toLowerCase().endsWith(".pdf");

  useEffect(() => {
    let cancelled = false;
    let url: string | null = null;
    setSrc(null);
    if (isImage) {
      url = URL.createObjectURL(file);
      setSrc(url);
    } else if (isPdf) {
      pdfFirstPage(file)
        .then((dataUrl) => {
          if (!cancelled) setSrc(dataUrl);
        })
        .catch(() => undefined);
    }
    return () => {
      cancelled = true;
      if (url) URL.revokeObjectURL(url);
    };
  }, [file, isImage, isPdf]);

  const ext = file.name.includes(".") ? file.name.split(".").pop()!.toUpperCase().slice(0, 4) : "?";
  return (
    <div
      className={`flex shrink-0 items-center justify-center overflow-hidden rounded-md border border-gray-200 bg-gray-50 dark:border-gray-700 dark:bg-gray-800 ${className}`}
      style={{ width: size, height: size }}
    >
      {src ? (
        <img src={src} alt="" className="h-full w-full object-cover object-top" />
      ) : (
        <span className="text-theme-xs font-medium text-gray-500 dark:text-gray-400">{ext}</span>
      )}
    </div>
  );
}

// pdfFirstPage renders page 1 of a PDF to a JPEG data URL. pdf.js is loaded on first use, so pages
// that never show a PDF do not pay for it.
async function pdfFirstPage(file: File): Promise<string> {
  const pdfjs = await import("pdfjs-dist");
  if (!pdfjs.GlobalWorkerOptions.workerSrc) {
    pdfjs.GlobalWorkerOptions.workerSrc = (await import("pdfjs-dist/build/pdf.worker.min.mjs?url")).default;
  }
  const task = pdfjs.getDocument({ data: new Uint8Array(await file.arrayBuffer()) });
  try {
    const doc = await task.promise;
    const page = await doc.getPage(1);
    const base = page.getViewport({ scale: 1 });
    const viewport = page.getViewport({ scale: PDF_RENDER_SIDE / Math.max(base.width, base.height) });
    const canvas = document.createElement("canvas");
    canvas.width = Math.ceil(viewport.width);
    canvas.height = Math.ceil(viewport.height);
    await page.render({ canvas, viewport }).promise;
    return canvas.toDataURL("image/jpeg", 0.8);
  } finally {
    void task.destroy();
  }
}
