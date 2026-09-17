import { useEffect, useState } from "react";
import {
  getEvidenceObjectUrl,
  listCaseCodifications,
  type CaseCodification,
} from "../services/cases";
import { cropImage } from "../utils/cropImage";

export interface CodificationThumbnail {
  codification: CaseCodification;
  /** Object URL of the trace's box cropped out of its evidence image, at the
   * box's own pixel size -- good enough to both tile and zoom into. */
  thumbnailUrl: string;
}

/** The analyst-facing label for one codification -- evidence sequence, trace
 * sequence and codification sequence, e.g. "1-3-1". */
export function codificationLabel(codification: CaseCodification): string {
  return `${codification.evidenceSequence}-${codification.traceSequence}-${codification.sequence}`;
}

/** Loads every codification of a case and crops each one's face out of its
 * evidence image. Each evidence image is downloaded once, however many
 * codifications sit on it. Bump `refreshKey` to reload. */
export function useCodificationThumbnails(caseId: string, refreshKey: number) {
  const [items, setItems] = useState<CodificationThumbnail[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    let cancelled = false;
    const objectUrls: string[] = [];

    async function load() {
      setLoading(true);
      setError("");
      try {
        const codifications = await listCaseCodifications(caseId);
        const evidenceUrls = new Map<number, string>();
        const results: CodificationThumbnail[] = [];

        for (const codification of codifications) {
          if (cancelled) return;

          let evidenceUrl = evidenceUrls.get(codification.evidenceFileId);
          if (!evidenceUrl) {
            evidenceUrl = await getEvidenceObjectUrl(
              caseId,
              codification.evidenceFileId,
            );
            evidenceUrls.set(codification.evidenceFileId, evidenceUrl);
            objectUrls.push(evidenceUrl);
          }

          const thumbnailUrl = await cropImage(evidenceUrl, {
            x1: codification.boxX1,
            y1: codification.boxY1,
            x2: codification.boxX2,
            y2: codification.boxY2,
          });
          objectUrls.push(thumbnailUrl);
          results.push({ codification, thumbnailUrl });
        }

        if (!cancelled) setItems(results);
      } catch (err) {
        if (!cancelled) {
          setError(
            err instanceof Error ? err.message : "Could not load codifications",
          );
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    }

    load();
    return () => {
      cancelled = true;
      for (const url of objectUrls) URL.revokeObjectURL(url);
    };
  }, [caseId, refreshKey]);

  return { items, loading, error };
}
