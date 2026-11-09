import { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { Map as MapLibreMap, Marker, NavigationControl } from "maplibre-gl";
import "maplibre-gl/dist/maplibre-gl.css";
import { TILE_ATTRIBUTION, TILE_URL } from "./tiles";

interface Props {
  lon: number;
  lat: number;
  /** The location's precision (EXACT, GEOCODED, STREET, MUNICIPALITY): sets the zoom, and a
   * point that is not exact is labelled as approximate. */
  precision?: string;
}

const zoomFor: Record<string, number> = { EXACT: 16, GEOCODED: 16, STREET: 15, NEIGHBOURHOOD: 14, MUNICIPALITY: 11 };

// One occurrence's point on a small read-only map, as the case page's occurrence card shows it.
// Scroll zoom is off so the page scrolls past it; the buttons zoom.
export default function OccurrencePointMap({ lon, lat, precision }: Props) {
  const { t } = useTranslation();
  const container = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!container.current) return;
    const m = new MapLibreMap({
      container: container.current,
      style: {
        version: 8,
        sources: { base: { type: "raster", tiles: [TILE_URL], tileSize: 256, attribution: TILE_ATTRIBUTION, maxzoom: 19 } },
        layers: [{ id: "base", type: "raster", source: "base" }],
      },
      center: [lon, lat],
      zoom: zoomFor[precision ?? ""] ?? 15,
      scrollZoom: false,
    });
    m.addControl(new NavigationControl({ showCompass: false }), "top-right");
    new Marker({ color: "#465fff" }).setLngLat([lon, lat]).addTo(m);
    // Re-measure when the layout changes, or part of the map stays blank (see OccurrenceMapView).
    const resize = new ResizeObserver(() => m.resize());
    resize.observe(container.current);
    return () => {
      resize.disconnect();
      m.remove();
    };
  }, [lon, lat, precision]);

  const approximate = precision && precision !== "EXACT";
  return (
    <div className="flex flex-col gap-1.5">
      <div
        ref={container}
        className="h-[260px] w-full overflow-hidden rounded-xl border border-gray-200 dark:border-gray-800"
        role="region"
        aria-label={t("caseOccurrence.mapLabel")}
      />
      {approximate && (
        <p className="text-theme-xs text-gray-500 dark:text-gray-400">
          {t("caseOccurrence.approximate", { precision: t(`precision.${precision}`) })}
        </p>
      )}
    </div>
  );
}
