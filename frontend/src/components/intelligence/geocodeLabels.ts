import type { TFunction } from "i18next";

// What the geocoder said about a location, for display: its source's credit (IBGE's census list
// and OpenStreetMap each require theirs) and what to check. See geocode.Result's Source and Flags.

/** The credit for a geocoder source ("cnefe2022:…", "osm:…", "ibge:…"), or "" for none known. */
export function geocodeSourceLabel(t: TFunction, source?: string): string {
  const kind = source?.startsWith("cnefe") ? "cnefe" : source?.split(":")[0];
  return kind === "cnefe" || kind === "osm" || kind === "ibge" ? t(`geocodeSource.${kind}`) : "";
}

/** Each flag in words. */
export function geocodeFlagLabels(t: TFunction, flags?: string[]): string[] {
  return (flags ?? []).map((f) => t(`geocodeFlag.${f}`, f));
}
