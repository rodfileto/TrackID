# Geocoding: the census address list in core, Nominatim optional

Proposed 2026-11-10. **All four phases are done** the same day (`geocode.Layered`,
`cmd/load-cnefe`, migrations 024 and 025, `CNEFE.Reverse`, `near`, the picker's flags) and were
tested on synthetic addresses and on IBGE's real Distrito Federal file (1.3 million rows, loaded in
16 s into about 170 MB; a click finds its address in under 1 ms), then measured on Rio de
Janeiro's (phase 4, results below). The design was decided the same day: IBGE's
census address list (CNEFE) becomes core's first source for addresses, both address → point and
point → address, with a Nominatim server optional behind it. The questions at the end are about
details.

## What is there now

*(This section describes core before phase 1.)*

- **One geocoder, Nominatim.** `geocode.Geocoder` has `Search` and `Reverse`. Its only
  implementation is `geocode.Nominatim`, built once in `server.Run` from `GEOCODER_*`. An empty
  `GEOCODER_URL` means OpenStreetMap's **public** instance.
- **What uses it.** The occurrence form's location picker (`LocationPicker.tsx`) uses it for
  address search, on the button only (`/geocode/search`), and to fill in the address of a point
  clicked on the map (`/geocode/reverse`). Importers with only an address use it too.
- **What doesn't.** Dropping a point on the map needs no geocoder: the point is kept with
  precision `EXACT`, and only the address fill-in fails, with a message. The map tiles come from
  the tile server, and nearby locations from TrackID's own database.
- **The problems.** Without a self-hosted Nominatim, a deployment sends crime addresses and
  clicked points to openstreetmap.org. The public instance's policy also forbids bulk geocoding.
  And OpenStreetMap often lacks house numbers in Brazil, and nearly always lacks addresses written
  by quadra/lote.
- **An organization's extension already solved part of this.** It loads CNEFE (IBGE, Censo 2022)
  into its own tables and, for an importer, tries CNEFE first and Nominatim second, with checks
  and flags (below). The code is generic: Brazilian address conventions applied to public data.
  It belongs in core.

## CNEFE in brief

IBGE's *Cadastro Nacional de Endereços para Fins Estatísticos* from Censo 2022: every address
the census visited, with coordinates, published per state as a zipped CSV. It is public data;
results must credit "Fonte: IBGE, Censo Demográfico 2022" (added to `docs/MODELS.md`). It covers
Brazil only, and only addresses (no named places such as an airport). Coordinates come with a
level (`NV_GEO_COORD`); levels 1–3 place the address itself, and coarser levels are dropped. A
large state reduces to about 1 GB in Postgres, and loading one takes a few GB of memory, once.

## Decisions

1. **CNEFE is tables in TrackID's own database, not a server.** Two tables, loaded per state by
   `cmd/load-cnefe` (reloading a state replaces it):
   - `cnefe_addresses`: one row per number on a street (municipality, street key, number),
     averaged over the addresses at that number.
   - `cnefe_places`: addresses with no number but a quadra, conjunto, lote, casa or bloco
     (`QR 308 CONJUNTO 13 LOTE 8`, `RUA 24 QD 40 LT 14`), filed under the whole chain and each
     shorter part, per locality.

   Two additions to the extension's schema:
   - a `street` display column (the street as IBGE writes it, e.g. "Rua Doutor João Pinheiro"),
     since the key is reduced and unfit to show;
   - a PostGIS point with a GiST index, for the nearest-address query.
2. **Search goes through steps, most exact first, and the first result that agrees wins.**
   1. CNEFE exact number;
   2. CNEFE nearest number on the same street (within 50);
   3. CNEFE quadra/lote chain, then its first part;
   4. Nominatim: number + street, then street, then free text when there is no street;
   5. the municipality, from core's `municipalities` (its centroid), so this step needs no
      server either.

   This is `geocode.Layered`, a `Geocoder` built from the sources configured, so callers don't
   change.
3. **Every result is checked against the address.** A result in another municipality or state
   than the address names is discarded (`agrees`). A geocoder will gladly return a same-named
   street in another city, which would put a false point on the map.
4. **Results say how sure they are.** `geocode.Result` gains `Flags` (a list of reasons to
   check the point):
   - `NEAREST_NUMBER`: the number wasn't found, so the point is the nearest on the street;
   - `STREET_ADJUSTED`: the street's spelling differs from the address's;
   - `LOCATOR_PARTIAL`: only part of a quadra/lote chain was found;
   - `STREET_ONLY`: the address has a number, the point only the street;
   - `MUNICIPALITY_ONLY`: only the municipality was found, so the point is its centroid;
   - `NAMED_PLACE`: found by name, with no city to confirm it;
   - `SOURCES_DISAGREE`: CNEFE and Nominatim are more than 2 km apart, when both answer.

   `Source` says where the point came from (`cnefe2022:<municipality>:<key>` or `osm:way/…`). The
   picker shows the flags beside a result; importers store them with the location.
5. **Address parsing is core's, for all of Brazil.** `geocode.ParseAddress` splits free text into
   street, number, quadra/lote chain, municipality and UF. It expands street abbreviations ("R.",
   "AV.", "DR.", …) and strips a postcode. It recognises all 27 states by name and by UF, not
   one, and the Distrito Federal's regions (Ceilândia/DF) as Brasília. This is what lets the
   picker's single search box reach CNEFE.
6. **Point → address: the nearest CNEFE address within a limit, then Nominatim.** A click
   returns the nearest CNEFE address within 50 m (measured in phase 4; proposed as 100 m): its street and number, and the locality as
   neighbourhood. Further than that it returns no address rather than a wrong one, then asks
   Nominatim if configured. In cities a click lands on the building or a neighbour. In rural
   areas, or anywhere without census addresses nearby, the user types the address, as now when
   the geocoder is down.
7. **Nominatim becomes optional, and the public instance opt-in.** `GEOCODER_URL` empty means
   **no** Nominatim, not the public one. Using the public instance takes
   `GEOCODER_URL=public`, and the server logs that addresses leave the network. CNEFE plus the
   municipality step still work without any server. Commands that geocode in bulk refuse the
   public instance (its policy forbids that), as the extension's importer already does. This
   changes today's default, so the CHANGELOG and `.env.example` say so.
8. **The picker biases a search by the map's view.** A search with no municipality in it
   (`Rua Goiás 120`) is looked up in the municipality at the map's centre: the API takes
   `near=lon,lat` and finds the municipality in `municipalities`. Without this, CNEFE couldn't
   answer most searches, since people rarely type the city.
9. **Production plan.** `deploy/` documents loading CNEFE as a step after install (`load-cnefe`
   per state the institute works in). The `nominatim` profile moves to phase 5: optional, for
   named places, streets without census addresses, and other countries.

## What phase 1 changed from the design

- **Numbers are keyed by locality too.** One municipality can have streets of the same name in
  different localities. In the Distrito Federal, 1% of the numbered keys (167 of 17,531) put
  such streets more than a kilometre apart, and averaging them would place the address between
  the two. An address that names its locality gets that one. One that names none gets the point
  only when every locality's is the same place (within 500 m); otherwise CNEFE gives no answer
  and the search goes on.
- **No default state.** An address with no state finds its municipality only when one
  municipality has that name; otherwise it needs the state (or, in phase 2, the map's view).
- **The parser** reads every state by code and by name, roads of every state (SP-330), more street
  types (viela, beco, ladeira, largo, vicinal, passagem, servidão, caminho), and the number after a
  comma first, since a street's name can end in a number (Rua 24, Vicinal VC 201).
- **The DF's `EDF` street type** is dropped from numbered streets as from places: it only says
  the street field is a sector.
- **Reverse stays the map server's** until phase 2.

## What phase 2 changed from the design

- **Places have a point too** (migration 025), so a click in the Distrito Federal finds its quadra
  and lote, not only numbered streets. The nearest row wins, the longer key on a tie, so a click on
  a lote gets the lote rather than its conjunto. A quadra or conjunto alone is `STREET` precision.
- **A municipality guessed from the map is never the answer.** When `near` supplies the
  municipality, the census list and the map server's number-and-street steps use it; if they find
  nothing, the map server's free-text search runs as if there were no `near` (so "Aeroporto de
  Confins" is still found from Belo Horizonte), and the municipality's centre is not returned.
- **The picker** shows each result's precision, source credit and flags, and, after a click or a
  pick, where the address came from and what to check.

## Phases

### Phase 1: CNEFE and the steps in core

- Migration `024_cnefe.sql` (two tables, display street, point and index) and `cmd/load-cnefe`.
- In `geocode`: `ParseAddress`, `StreetKey`, the CNEFE lookup (exact, nearest number, places), the
  municipality step, `Layered`, `agrees` and `Flags`, ported from the extension and generalised
  (all UFs, display street).
- `server.Run` builds `Layered` from what is configured: CNEFE when loaded, Nominatim when
  `GEOCODER_URL` is set, and always the municipalities.
- Tests: the extension's parsing and place tests, plus a small synthetic CNEFE CSV (a few
  invented streets in two municipalities, quadra/lote rows, a coarse-level row to drop) that
  covers exact, nearest, partial, wrong-city and no-result cases.

### Phase 2: point → address and the picker

- `Reverse` on CNEFE (nearest within a limit, measured in phase 4), the `near` bias on search (decision 8), and the
  picker showing flags and the source (CNEFE or OpenStreetMap, each with its attribution).

### Phase 3: defaults and documentation

- `GEOCODER_URL` empty means none, and `public` is opt-in (decision 7). Bulk commands refuse the
  public instance.
- `docs/MODELS.md` gains IBGE's attribution, `.env.example` and the deployment guide gain the
  CNEFE step, and the CHANGELOG notes the default change.

### Phase 4: measure

- How often each step answers, and how far CNEFE and Nominatim points fall apart, on synthetic
  addresses (trackid-sim's geography, once the simulator follows core again). The output is
  counts only, so it can be published.

## Phase 4: what was measured

`trackid-sim measure-geocoding` (seed 1), on IBGE's file for the state of Rio de Janeiro loaded
with `load-cnefe` (8,962,200 rows, 87 s, 2.7 GB of memory at most) and the simulator's geography,
the city of Rio de Janeiro (781,624 numbered census addresses; its boundary taken from the
simulator's 41 police-station areas). No map server: nothing was sent outside, and the
OpenStreetMap points compared are those the simulator cached when it built its bank branches.
Counts only.

**The reverse limit.** A click on a building whose address is not in the list gets the nearest
one that is. Each of 50,000 census addresses was left out in turn and the nearest other one
found. How often it is on the same street, by how far it is:

| nearest other address | addresses | on the same street |
|---|---:|---:|
| ≤ 10 m | 36,508 | 88% |
| 10–25 m | 11,726 | 83% |
| 25–50 m | 1,413 | 65% |
| 50–75 m | 208 | 58% |
| 75–100 m | 70 | 50% |
| 100–150 m | 50 | 46% |
| farther | 25 | 64% |

Past 50 m the street is right about half the time, so the limit is **50 m** (`ReverseMeters`).
For the simulator's own occurrence points (2,000, placed as it places occurrences), the nearest
census address is within 50 m for 56% and within 100 m for 67%: the rest are parks, hills,
water and roads, where the user types the address.

**Search**, on 2,000 census addresses written six ways. Steps: the number (`exact`), the nearest
number (`nearest`), a street spelled differently (`adjusted`), a quadra/lote (`place`), the
municipality alone, or nothing. Distances are the census list's answers from the address's
point.

| written | exact | nearest | adjusted | place | municipality | none | ≤ 50 m | 50–200 m | 0.2–1 km | farther |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| in full, with the neighbourhood | 1,983 | 1 | 0 | 6 | 10 | 0 | 1,983 | 3 | 3 | 1 |
| abbreviated, no neighbourhood | 1,753 | 5 | 0 | 6 | 236 | 0 | 1,738 | 12 | 10 | 4 |
| street and number only (`near`) | 1,762 | 0 | 0 | 6 | 0 | 232 | 1,749 | 10 | 9 | 0 |
| a number not in the list | 93 | 1,892 | 0 | 6 | 9 | 0 | 1,722 | 214 | 44 | 11 |
| two letters swapped | 193 | 0 | 1,687 | 6 | 114 | 0 | 1,880 | 3 | 3 | 0 |
| no number | 7 | 6 | 0 | 6 | 1,981 | 0 | 6 | 4 | 4 | 5 |

An address with no neighbourhood on a street whose name several neighbourhoods have (Rio has
dozens of "Rua A" and "Travessa União") gets the municipality, or nothing when the
municipality was only guessed from the map: by design. The far answers left are mostly streets
literally called "Rua Sem Denominação" or "Rua Projetada A", many to a neighbourhood.

The first run of this measurement found five defects, fixed in the same change: one-letter
street names ("Rua A") and abbreviations (Tv., Estr., Al., Pç.) and types (Via, Vila, Escadaria,
Entrada, Acesso, Travessia) the parser didn't read; a swap of two letters counted as two slips;
a number taken from a same-named street in another neighbourhood when the named one lacks it;
a locality named like the municipality ("Rio de Janeiro") or like the street (Rua Laranjeiras)
taken for the neighbourhood the address names. Fully written addresses more than 1 km off went
from 45 to 1 in 2,000.

**The census list against OpenStreetMap**, on the simulator's 481 bank branches (addresses from
the Banco Central's register; OpenStreetMap placed 125 by number, 285 by street and 71 by
neighbourhood). The census list answered 343 (267 by number, 71 by the nearest number, 5 by
another spelling) and gave the municipality for 138. Where both placed the number:

| apart | branches |
|---:|---:|
| ≤ 50 m | 55 |
| 50–200 m | 25 |
| 0.2–1 km | 2 |
| 1–2 km | 1 |
| farther | 3 |

So the 2 km at which a result says `SOURCES_DISAGREE` flags 3 of 86 addresses both sources
placed by number.

## Organizations extending core

An extension that already has its own CNEFE tables reloads the states into core's tables with
core's `load-cnefe`, since the display street column is new. It then drops its own tables and
calls `geocode.Layered` / `geocode.ParseAddress` instead of its copies, keeping only what is its
own (its importer, its corrections). Its version of core must include this plan's phase 1 first.

## Decided questions (2026-11-10)

1. **The reverse-lookup limit is measured,** in phase 4, not guessed or left to each deployment.
   It is 50 m (phase 4).
2. **The new default is accepted.** `GEOCODER_URL` empty means no Nominatim before `1.0`; the
   CHANGELOG says so.
3. **The dev stack loads no CNEFE state by default.** A script fetches and loads one state on
   request.
