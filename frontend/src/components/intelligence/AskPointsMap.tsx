import { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { LngLatBounds, Map as MapLibreMap, Marker, NavigationControl, Popup } from "maplibre-gl";
import "maplibre-gl/dist/maplibre-gl.css";
import { tileAttribution, tileUrl } from "./tiles";

export interface AskPoint {
  lat: number;
  lon: number;
  label: string;
  /** Marks the place a search was made around, apart from what it found. */
  center?: boolean;
}

// The points of an AI mode answer on a small read-only map. Scroll zoom is off so the conversation
// scrolls past it; the buttons zoom.
export default function AskPointsMap({ points }: { points: AskPoint[] }) {
  const { t } = useTranslation();
  const container = useRef<HTMLDivElement>(null);
  const key = JSON.stringify(points);

  useEffect(() => {
    if (!container.current) return;
    const list = JSON.parse(key) as AskPoint[];
    if (list.length === 0) return;
    const m = new MapLibreMap({
      container: container.current,
      style: {
        version: 8,
        sources: { base: { type: "raster", tiles: [tileUrl()], tileSize: 256, attribution: tileAttribution(), maxzoom: 19 } },
        layers: [{ id: "base", type: "raster", source: "base" }],
      },
      center: [list[0].lon, list[0].lat],
      zoom: 15,
      scrollZoom: false,
    });
    m.addControl(new NavigationControl({ showCompass: false }), "top-right");
    const bounds = new LngLatBounds();
    for (const p of list) {
      const popup = new Popup({ closeButton: false }).setText(p.label);
      new Marker({ color: p.center ? "#f79009" : "#465fff" }).setLngLat([p.lon, p.lat]).setPopup(popup).addTo(m);
      bounds.extend([p.lon, p.lat]);
    }
    if (list.length > 1) m.fitBounds(bounds, { padding: 40, maxZoom: 16, animate: false });
    // Re-measure when the layout changes, or part of the map stays blank (see OccurrenceMapView).
    const resize = new ResizeObserver(() => m.resize());
    resize.observe(container.current);
    return () => {
      resize.disconnect();
      m.remove();
    };
  }, [key]);

  return (
    <div
      ref={container}
      className="h-[240px] w-full overflow-hidden rounded-xl border border-gray-200 dark:border-gray-800"
      role="region"
      aria-label={t("ask.mapLabel")}
    />
  );
}
