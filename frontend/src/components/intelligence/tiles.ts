import { setWorkerUrl } from "maplibre-gl";

// The base map. OpenStreetMap's own tile servers allow light use only
// (https://operations.osmfoundation.org/policies/tiles/): a deployment sets
// VITE_MAP_TILE_URL to its own or a contracted tile service. Whatever serves
// OSM data must credit its contributors.
export const TILE_URL: string =
  import.meta.env.VITE_MAP_TILE_URL ??
  "https://tile.openstreetmap.org/{z}/{x}/{y}.png";
export const TILE_ATTRIBUTION: string =
  import.meta.env.VITE_MAP_TILE_ATTRIBUTION ??
  '© <a href="https://www.openstreetmap.org/copyright" target="_blank" rel="noopener">OpenStreetMap</a> contributors';

// MapLibre's Web Worker, served by vite.config.ts's maplibreWorker plugin in dev
// and in the build. Every map component imports this module.
setWorkerUrl("/maplibre/maplibre-gl-worker.mjs");
