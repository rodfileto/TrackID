import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { clock, type VideoLabFace } from "../../services/videolab";
import { personColor, sameFace, type VideoPerson } from "./faces";

interface Props {
  persons: VideoPerson[];
  selected?: { track: number; face: VideoLabFace } | null;
  onSelect: (track: number, face: VideoLabFace) => void;
  onFaceDoubleClick?: (track: number, face: VideoLabFace) => void;
  /** Buttons for a person, under its details. */
  actions?: (person: VideoPerson) => ReactNode;
}

/** The persons found in a video, each with the best face large (★) and a strip of all its faces. */
export default function PersonFaces({ persons, selected, onSelect, onFaceDoubleClick, actions }: Props) {
  const { t } = useTranslation();
  return (
    <div className="space-y-5">
      {persons.map((p) => (
        <div key={p.track} className="rounded-lg border border-gray-200 p-3 dark:border-gray-800">
          <div className="flex flex-wrap items-start gap-4">
            <button
              type="button"
              onClick={() => onSelect(p.track, p.best)}
              className="shrink-0 rounded-lg border-2 p-1"
              style={{ borderColor: personColor(p.track) }}
              title={t("videoLab.bestHint")}
            >
              <img src={p.best.crop} alt={t("videoLab.person", { n: p.track + 1 })} className="h-32 w-32 rounded object-cover" />
            </button>
            <div className="min-w-0 flex-1 text-sm text-gray-700 dark:text-gray-300">
              <p className="font-medium" style={{ color: personColor(p.track) }}>
                {t("videoLab.person", { n: p.track + 1 })}
              </p>
              <p>{t("videoLab.span", { first: clock(p.firstMs), last: clock(p.lastMs), count: p.faces.length })}</p>
              <p>
                ★ {t("videoLab.bestAt", { time: clock(p.best.timestampMs) })}
                {p.best.frame !== undefined && ` · ${t("videoLab.frameNumber", { n: p.best.frame })}`}
                {` · ${t("videoLab.confidence", { value: p.best.confidence.toFixed(3) })}`}
                {` · ${t("videoLab.sharpness", { value: p.best.blur.toFixed(0) })}`}
              </p>
              {actions && <div className="mt-2 flex flex-wrap gap-2">{actions(p)}</div>}
            </div>
          </div>
          <div className="mt-3 flex gap-2 overflow-x-auto pb-1">
            {p.faces.map((f, i) => {
              const isSel = selected?.track === p.track && sameFace(selected.face, f);
              return (
                <button
                  key={i}
                  type="button"
                  onClick={() => onSelect(p.track, f)}
                  onDoubleClick={onFaceDoubleClick ? () => onFaceDoubleClick(p.track, f) : undefined}
                  className={`relative shrink-0 rounded border-2 ${isSel ? "" : "border-transparent"}`}
                  style={isSel ? { borderColor: personColor(p.track) } : undefined}
                  title={`${clock(f.timestampMs)}${f.frame !== undefined ? ` · #${f.frame}` : ""} · ${t("videoLab.confidence", { value: f.confidence.toFixed(3) })} · ${t("videoLab.sharpness", { value: f.blur.toFixed(0) })}${f.embedded ? "" : ` · ${t("videoLab.attributed")}`}`}
                >
                  <img src={f.crop} alt={clock(f.timestampMs)} className={`h-16 w-16 rounded object-cover ${f.embedded ? "" : "opacity-80"}`} />
                  {f.best && <span className="absolute right-0.5 top-0.5 rounded bg-warning-500 px-1 text-[10px] font-bold text-white">★</span>}
                  <span className="block text-center font-mono text-[10px] text-gray-500">{clock(f.timestampMs).slice(3, 8)}</span>
                </button>
              );
            })}
          </div>
        </div>
      ))}
    </div>
  );
}
