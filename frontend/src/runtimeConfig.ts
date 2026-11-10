import { apiBaseUrl } from "./services/api";

/** What the server tells the frontend at startup (GET /api/config), so one build fits every deployment. */
export interface RuntimeConfig {
  mapTileUrl?: string;
  mapTileAttribution?: string;
}

let current: RuntimeConfig = {};

/**
 * Reads the configuration before the app renders. A server that can't answer leaves the defaults in
 * place rather than keeping the app from starting.
 */
export async function loadRuntimeConfig(): Promise<void> {
  try {
    const response = await fetch(`${apiBaseUrl}/config`, { headers: { Accept: "application/json" } });
    if (response.ok) current = (await response.json()) as RuntimeConfig;
  } catch {
    // Keep the defaults.
  }
}

export function runtimeConfig(): RuntimeConfig {
  return current;
}
