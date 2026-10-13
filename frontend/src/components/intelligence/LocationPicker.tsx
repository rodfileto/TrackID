import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  LngLatBounds,
  Map as MapLibreMap,
  Marker,
  NavigationControl,
} from "maplibre-gl";
import "maplibre-gl/dist/maplibre-gl.css";
import {
  geocodeReverse,
  geocodeSearch,
  nearbyLocations,
  type GeocodeResult,
  type NearbyLocation,
  type PlaceInput,
} from "../../services/occurrences";
import { TILE_ATTRIBUTION, TILE_URL } from "./tiles";

interface Props {
  value: PlaceInput;
  onChange: (place: PlaceInput) => void;
}

// Where an occurrence happened: click the map to drop a point (its address is
// filled by reverse geocoding), or search an address and pick a result. Existing
// locations within 50 m are offered for reuse, so a place hit again stays one
// location. The address search runs on the button only: the public Nominatim
// forbids search-as-you-type.
export default function LocationPicker({ value, onChange }: Props) {
  const { t } = useTranslation();
  const container = useRef<HTMLDivElement>(null);
  const map = useRef<MapLibreMap | null>(null);
  const marker = useRef<Marker | null>(null);
  const latest = useRef({ value, onChange });
  latest.current = { value, onChange };

  const [query, setQuery] = useState("");
  const [results, setResults] = useState<GeocodeResult[] | null>(null);
  const [searching, setSearching] = useState(false);
  const [nearby, setNearby] = useState<NearbyLocation[]>([]);
  const [message, setMessage] = useState("");

  // A point chosen on the map or from a search: reverse-geocode a click, then
  // look for existing locations around it.
  async function choosePoint(lon: number, lat: number, from?: GeocodeResult) {
    const base: PlaceInput = {
      ...latest.current.value,
      existingId: undefined,
      lon,
      lat,
      precision: from ? from.precision : "EXACT",
    };
    latest.current.onChange(base);
    setMessage("");
    try {
      const address = from ?? (await geocodeReverse(lon, lat));
      if (address) {
        latest.current.onChange({
          ...base,
          address: [address.street, address.houseNumber].filter(Boolean).join(", ") || address.displayName,
          neighbourhood: address.neighbourhood ?? base.neighbourhood,
        });
      }
    } catch {
      setMessage(t("occurrenceForm.geocoderUnavailable"));
    }
    nearbyLocations(lon, lat).then(setNearby).catch(() => setNearby([]));
  }

  useEffect(() => {
    if (!container.current) return;
    const m = new MapLibreMap({
      container: container.current,
      style: {
        version: 8,
        sources: { base: { type: "raster", tiles: [TILE_URL], tileSize: 256, attribution: TILE_ATTRIBUTION, maxzoom: 19 } },
        layers: [{ id: "base", type: "raster", source: "base" }],
      },
      center: [-43.4, -22.92],
      zoom: 10,
    });
    m.addControl(new NavigationControl({ showCompass: false }), "top-right");
    m.on("click", (e) => choosePoint(e.lngLat.lng, e.lngLat.lat));
    map.current = m;
    // Re-measure when the layout changes, or part of the map stays blank (see OccurrenceMapView).
    const resize = new ResizeObserver(() => m.resize());
    resize.observe(container.current);
    return () => {
      resize.disconnect();
      m.remove();
      map.current = null;
    };
    // choosePoint reads the latest props through a ref.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Keep the marker on the chosen point.
  useEffect(() => {
    const m = map.current;
    if (!m) return;
    if (value.lon === undefined || value.lat === undefined) {
      marker.current?.remove();
      marker.current = null;
      return;
    }
    if (!marker.current) marker.current = new Marker({ color: "#465fff" }).setLngLat([value.lon, value.lat]).addTo(m);
    else marker.current.setLngLat([value.lon, value.lat]);
  }, [value.lon, value.lat]);

  // Not a <form>: the picker sits inside the occurrence form, and a nested form
  // would make Search (or Enter) submit the occurrence instead.
  async function search() {
    if (!query.trim()) return;
    setSearching(true);
    setMessage("");
    try {
      const found = await geocodeSearch(query.trim());
      setResults(found);
      if (found.length > 0 && map.current) {
        const b = new LngLatBounds();
        found.forEach((r) => b.extend([r.lon, r.lat]));
        map.current.fitBounds(b, { padding: 60, maxZoom: 16 });
      }
    } catch {
      setMessage(t("occurrenceForm.geocoderUnavailable"));
    } finally {
      setSearching(false);
    }
  }

  function pickResult(r: GeocodeResult) {
    setResults(null);
    map.current?.flyTo({ center: [r.lon, r.lat], zoom: 16 });
    choosePoint(r.lon, r.lat, r);
  }

  function reuse(l: NearbyLocation) {
    onChange({
      existingId: l.id,
      lon: l.lon,
      lat: l.lat,
      precision: l.precision,
      address: l.address,
      neighbourhood: l.neighbourhood,
      name: l.name,
      placeType: l.placeType,
    });
  }

  const inputClass =
    "h-11 w-full rounded-lg border border-gray-300 bg-transparent px-4 text-sm text-gray-800 shadow-theme-xs placeholder:text-gray-400 focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90";

  return (
    <div className="flex flex-col gap-3">
      <div className="flex gap-2" role="search">
        <input
          className={inputClass}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              search();
            }
          }}
          placeholder={t("occurrenceForm.addressPlaceholder")}
          aria-label={t("occurrenceForm.addressSearch")}
        />
        <button
          type="button"
          onClick={search}
          disabled={searching || !query.trim()}
          className="h-11 shrink-0 rounded-lg bg-brand-500 px-4 text-sm font-medium text-white hover:bg-brand-600 disabled:opacity-50"
        >
          {searching ? t("occurrenceForm.searching") : t("occurrenceForm.search")}
        </button>
      </div>
      {results && (
        <ul className="flex flex-col divide-y divide-gray-100 rounded-lg border border-gray-200 dark:divide-gray-800 dark:border-gray-800">
          {results.length === 0 && (
            <li className="px-3 py-2 text-sm text-gray-500 dark:text-gray-400">{t("occurrenceForm.noResults")}</li>
          )}
          {results.map((r) => (
            <li key={`${r.lon},${r.lat}`}>
              <button
                type="button"
                onClick={() => pickResult(r)}
                className="w-full px-3 py-2 text-left text-sm text-gray-700 hover:bg-gray-50 dark:text-gray-300 dark:hover:bg-white/[0.03]"
              >
                {r.displayName}
              </button>
            </li>
          ))}
        </ul>
      )}
      {message && <p className="text-sm text-warning-600 dark:text-warning-400">{message}</p>}

      <div
        ref={container}
        className="h-[360px] w-full overflow-hidden rounded-xl border border-gray-200 dark:border-gray-800"
        role="region"
        aria-label={t("occurrenceForm.mapHint")}
      />
      <p className="text-theme-xs text-gray-500 dark:text-gray-400">{t("occurrenceForm.mapHint")}</p>

      {nearby.length > 0 && (
        <div className="flex flex-col gap-2 rounded-lg border border-gray-200 p-3 dark:border-gray-800">
          <span className="text-sm font-medium text-gray-700 dark:text-gray-300">{t("occurrenceForm.nearbyTitle")}</span>
          {nearby.map((l) => (
            <button
              key={l.id}
              type="button"
              onClick={() => reuse(l)}
              aria-pressed={value.existingId === l.id}
              className={`rounded-lg border px-3 py-2 text-left text-sm ${
                value.existingId === l.id
                  ? "border-brand-500 bg-brand-50 text-brand-700 dark:bg-brand-500/10 dark:text-brand-300"
                  : "border-gray-200 text-gray-700 hover:bg-gray-50 dark:border-gray-800 dark:text-gray-300 dark:hover:bg-white/[0.03]"
              }`}
            >
              {[l.name, l.address, l.neighbourhood].filter(Boolean).join(" · ") || t("occurrenceForm.unnamedPlace")}
              <span className="ml-2 text-gray-400">
                {t("occurrenceForm.nearbyMeta", { distance: Math.round(l.distanceM), count: l.occurrenceCount })}
              </span>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
