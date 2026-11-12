# Main areas of presence

Proposed 2026-11-12, not built. The person and cluster pages now draw where the person or cluster appears: the
occurrences of the cases it has traces in (`api/presence_handler.go`, `PresenceMapCard`). That is every point. This plan
adds the summary an analyst asks next: **where is the main area, and is there more than one?**

## What a point is, and is not

- A point is **where an occurrence happened**, through a case the person or cluster has a trace in. It is not where the
  person lives or was seen, and a case is where the institute was asked to examine, not where it all happened. The tool
  says so wherever it shows an area.
- Only occurrences with a real place count. Those with none, or only their unit's municipality (`JURISDICTION`,
  migration 030), are left out and counted, as the map does. `MUNICIPALITY`-precision points are the municipality's
  centre: they are not clustered by distance (they would pile up at one spot) but counted in the by-municipality view.
- The unit is the **occurrence**, not the case: two cases on one occurrence are one point. Weight is 1 each.
- A point's time is the occurrence's `occurred_from`, which for the imported scene laudos is the perícia's date when the
  crime's is not stated (decided 2026-11-12): fine for "which years", not for "which day".

## Method

Density-based clustering (DBSCAN) of the points, in PostGIS (`ST_ClusterDBSCAN`), with two parameters: a distance `eps`
and `minPts`.

| Option | For | Against |
|---|---|---|
| **DBSCAN** (chosen) | finds any number of areas of any shape, leaves isolated points out as noise, parameters mean something (a distance, a count) | needs `eps`; one value does not fit a street and a state |
| k-means | simple | needs the number of areas in advance, forces every point into one, so a lone far occurrence drags an area |
| Standard deviational ellipse | one summary of spread | hides two centres by averaging between them |
| Grid / hexagon counts (H3) | stable, comparable between people, no shape parameters | areas are cells, not places; border effects |
| Kernel density | smooth | already the heatmap; not discrete, no count per area |

- **Projection.** `eps` is in metres, so points are projected to SIRGAS 2000 / Brazil Polyconic (EPSG:5880), one
  projection for the whole country, not geography degrees and not a UTM zone per state.
- **Presets for `eps`**, not a free number by default: *quadra / bairro* 500 m, *cidade* 3 km, *região* 25 km, each with
  `minPts` 2. Two occurrences within `eps` make an area: people usually have a handful of cases, so a higher `minPts`
  would find nothing. The analyst can change both.
- **Per area**: its occurrences, weighted centre, a hull (`ST_ConcaveHull`, a convex hull for fewer than four points), the
  radius holding 80 % of its points, first and last date, the top types, and its share of the person's mapped
  occurrences. The **main area** is the one with the most occurrences, ties by the most recent.
- **By municipality**: the same occurrences counted by `municipality_code`, a bar list. It needs no parameters, works
  with `MUNICIPALITY`-precision points, and is the honest view when there are few points.

## What it must not claim

1. **Too few points.** Under three mapped occurrences it shows the points and no "main area".
2. **Where the institute works.** Cases come from where the PF investigates, so most areas fall where most
   occurrences are. For each area the tool also shows how much of *all* occurrences in the same area it is
   (a location quotient: the person's share there against the whole base's), so an area that is only where everything
   happens reads as such.
3. **Certain links.** A person reaches a case through a cluster, and a cluster is the matcher's (`AUTOMATIC`) or a
   person's (`REVIEWED`) judgement. The tool has a "reviewed links only" switch, and each area says how many of its
   occurrences are reviewed.
4. **Residence or routine.** The wording is "areas of occurrences", never "lives in" or "operates in".

## Interface

- On the person and cluster pages, beside the map: a distance preset, a "reviewed links only" switch and a period
  filter (by year).
- The map gets a second layer: each area's hull, shaded by size, its centre marked, the main area outlined; the points
  stay. The list beside it ranks the areas (count, share, period, quotient) and selecting one frames it on the map.
- The by-municipality bars sit under the list.
- Same card on both pages, as `PresenceMapCard` is.

## API

`GET /api/persons/:personId/presence-areas` and `GET /api/clusters/:clusterId/presence-areas`, with `eps`, `minPts`,
`reviewedOnly` and `year`. Intelligence only, the person's read logged as the others are. One engine,
`presence.Areas(points, eps, minPts)`, which takes the points and does not know where they come from, so it can also
serve the occurrence map (hotspots of all occurrences, by type group) and the AI mode.

## Phases

1. `presence` package: the SQL (`ST_ClusterDBSCAN` over the projected points, hull, radius, centre) and the municipality
   count; tests on made-up coordinates (two separated groups, one lone point, a single area, too few points, points on
   the same spot, a point just inside and just outside `eps`).
2. The two routes, the access-table test, and the reviewed-only and period filters.
3. The card: layer, ranked list, presets, municipality bars.
4. The location quotient and the reviewed counts.
5. The AI mode tool and, in the report layer, a sentence for a person's page article.

## Open

- Whether `reviewedOnly` is on by default for a person, as persons are enrolled identities, or off, as most links are
  automatic.
- Whether fingerprint cases join a person's map. They reach a person through their own clusters, so they would, but
  their places are mostly scene laudos' and the facial ones' are mostly unit municipalities: the map would look unlike
  the facial one. Not decided.
- The right `eps` presets for the institute's states (Minas Gerais is large; 25 km may be small).
