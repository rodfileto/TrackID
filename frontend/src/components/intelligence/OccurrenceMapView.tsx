import { useEffect, useRef } from "react";
import {
  LngLatBounds,
  Map as MapLibreMap,
  NavigationControl,
  ScaleControl,
  type ExpressionSpecification,
  type GeoJSONSource,
} from "maplibre-gl";
import "maplibre-gl/dist/maplibre-gl.css";
import type {
  OccurrenceFeature,
  OccurrenceMap,
} from "../../services/occurrences";
import { GROUP_COLORS, OTHER_COLOR } from "./groupColors";
import { TILE_ATTRIBUTION, TILE_URL } from "./tiles";

export type MapMode = "points" | "heatmap";

interface Props {
  data: OccurrenceMap | null;
  mode: MapMode;
  selectedId?: number;
  onSelect: (feature: OccurrenceFeature | null) => void;
}

const SOURCE = "occurrences";


function groupColorExpression(): ExpressionSpecification {
  const pairs = Object.entries(GROUP_COLORS).flat();
  return ["match", ["get", "group"], ...pairs, OTHER_COLOR] as unknown as ExpressionSpecification;
}

// Occurrences on a map of their locations: a point per occurrence coloured by
// group, or a heatmap of their density. Clicking a point selects it.
export default function OccurrenceMapView({
  data,
  mode,
  selectedId,
  onSelect,
}: Props) {
  const container = useRef<HTMLDivElement>(null);
  const map = useRef<MapLibreMap | null>(null);
  const ready = useRef(false);
  const latest = useRef({ data, mode, selectedId, onSelect });
  latest.current = { data, mode, selectedId, onSelect };

  useEffect(() => {
    if (!container.current) return;
    const m = new MapLibreMap({
      container: container.current,
      style: {
        version: 8,
        sources: {
          base: {
            type: "raster",
            tiles: [TILE_URL],
            tileSize: 256,
            attribution: TILE_ATTRIBUTION,
            maxzoom: 19,
          },
        },
        layers: [{ id: "base", type: "raster", source: "base" }],
      },
      // Rio de Janeiro until the data says otherwise.
      center: [-43.4, -22.92],
      zoom: 9.5,
    });
    m.addControl(new NavigationControl({ showCompass: false }), "top-right");
    m.addControl(new ScaleControl({ unit: "metric" }), "bottom-left");

    m.on("load", () => {
      m.addSource(SOURCE, {
        type: "geojson",
        data: { type: "FeatureCollection", features: [] },
      });
      m.addLayer({
        id: "heat",
        type: "heatmap",
        source: SOURCE,
        layout: { visibility: "none" },
        paint: {
          "heatmap-radius": ["interpolate", ["linear"], ["zoom"], 9, 14, 14, 30],
          "heatmap-intensity": ["interpolate", ["linear"], ["zoom"], 9, 0.8, 14, 2],
          "heatmap-opacity": 0.8,
        },
      });
      m.addLayer({
        id: "points",
        type: "circle",
        source: SOURCE,
        paint: {
          "circle-color": groupColorExpression(),
          "circle-radius": ["interpolate", ["linear"], ["zoom"], 9, 4, 15, 8],
          "circle-stroke-color": "#ffffff",
          "circle-stroke-width": 1,
          "circle-opacity": 0.9,
        },
      });
      m.addLayer({
        id: "selected",
        type: "circle",
        source: SOURCE,
        filter: ["==", ["get", "id"], -1],
        paint: {
          "circle-color": "rgba(0,0,0,0)",
          "circle-radius": ["interpolate", ["linear"], ["zoom"], 9, 9, 15, 14],
          "circle-stroke-color": "#111827",
          "circle-stroke-width": 3,
        },
      });
      m.on("click", "points", (e) => {
        const f = e.features?.[0];
        const id = f?.properties?.id;
        const feature = latest.current.data?.features.find((x) => x.properties.id === id);
        latest.current.onSelect(feature ?? null);
      });
      m.on("click", (e) => {
        if (m.queryRenderedFeatures(e.point, { layers: ["points"] }).length === 0) {
          latest.current.onSelect(null);
        }
      });
      m.on("mouseenter", "points", () => (m.getCanvas().style.cursor = "pointer"));
      m.on("mouseleave", "points", () => (m.getCanvas().style.cursor = ""));
      ready.current = true;
      apply(m, latest.current.data, true);
      applyMode(m, latest.current.mode);
      applySelection(m, latest.current.selectedId);
    });
    map.current = m;
    // MapLibre measures its container once; re-measure when the layout settles or changes
    // (lazy-loaded page, sidebar toggled, panel shown), or part of the map stays blank.
    const resize = new ResizeObserver(() => m.resize());
    resize.observe(container.current);
    return () => {
      resize.disconnect();
      ready.current = false;
      m.remove();
      map.current = null;
    };
  }, []);

  useEffect(() => {
    if (map.current && ready.current) apply(map.current, data, true);
  }, [data]);
  useEffect(() => {
    if (map.current && ready.current) applyMode(map.current, mode);
  }, [mode]);
  useEffect(() => {
    if (map.current && ready.current) applySelection(map.current, selectedId);
  }, [selectedId]);

  return (
    <div
      ref={container}
      className="h-[560px] w-full overflow-hidden rounded-xl border border-gray-200 dark:border-gray-800"
      role="region"
      aria-label="Occurrence map"
    />
  );
}

// apply shows data and, when fit, frames it. MapLibre's GeoJSON properties are
// flat, so arrays travel as JSON and the full feature is looked up on click.
function apply(m: MapLibreMap, data: OccurrenceMap | null, fit: boolean) {
  const source = m.getSource(SOURCE) as GeoJSONSource | undefined;
  if (!source) return;
  const features = data?.features ?? [];
  source.setData({
    type: "FeatureCollection",
    features: features.map((f) => ({
      type: "Feature",
      geometry: f.geometry,
      properties: { id: f.properties.id, group: f.properties.group },
    })),
  });
  if (fit && features.length > 0) {
    const bounds = new LngLatBounds();
    for (const f of features) bounds.extend(f.geometry.coordinates);
    m.fitBounds(bounds, { padding: 40, maxZoom: 14, duration: 0 });
  }
}

function applyMode(m: MapLibreMap, mode: MapMode) {
  m.setLayoutProperty("heat", "visibility", mode === "heatmap" ? "visible" : "none");
  m.setPaintProperty("points", "circle-opacity", mode === "heatmap" ? 0.25 : 0.9);
}

function applySelection(m: MapLibreMap, id?: number) {
  m.setFilter("selected", ["==", ["get", "id"], id ?? -1]);
}
