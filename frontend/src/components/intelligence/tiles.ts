import { setWorkerUrl } from "maplibre-gl";
import { runtimeConfig } from "../../runtimeConfig";

// The base map. OpenStreetMap's own tile servers allow light use only
// (https://operations.osmfoundation.org/policies/tiles/): a deployment sets
// MAP_TILE_URL on the server (served by /api/config) to its own or a contracted
// tile service. VITE_MAP_TILE_URL still sets a build's default. Whatever serves
// OSM data must credit its contributors.
const DEFAULT_TILE_URL = "https://tile.openstreetmap.org/{z}/{x}/{y}.png";
const DEFAULT_TILE_ATTRIBUTION =
  '© <a href="https://www.openstreetmap.org/copyright" target="_blank" rel="noopener">OpenStreetMap</a> contributors';

export function tileUrl(): string {
  return runtimeConfig().mapTileUrl || import.meta.env.VITE_MAP_TILE_URL || DEFAULT_TILE_URL;
}

export function tileAttribution(): string {
  return runtimeConfig().mapTileAttribution || import.meta.env.VITE_MAP_TILE_ATTRIBUTION || DEFAULT_TILE_ATTRIBUTION;
}

// MapLibre's Web Worker, served by vite.config.ts's maplibreWorker plugin in dev
// and in the build. Every map component imports this module.
setWorkerUrl("/maplibre/maplibre-gl-worker.mjs");
