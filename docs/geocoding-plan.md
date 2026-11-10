# Geocoding: the census address list in core, Nominatim optional

Proposed 2026-11-10. **Phase 1 is built** the same day (`geocode.Layered`, `cmd/load-cnefe`,
migration 024) and was tested on synthetic addresses and on IBGE's real Distrito Federal file
(1.3 million rows, loaded in 16 s into about 170 MB). Phases 2 to 4 are still proposals. The
design was decided the same day: IBGE's
census address list (CNEFE) becomes core's first source for addresses, both address → point and
point → address, with a Nominatim server optional behind it. The questions at the end are about
details.

## What is there now

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
   returns the nearest CNEFE address within 100 m: its street and number, and the locality as
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

- `Reverse` on CNEFE (nearest within 100 m), the `near` bias on search (decision 8), and the
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

## Organizations extending core

An extension that already has its own CNEFE tables reloads the states into core's tables with
core's `load-cnefe`, since the display street column is new. It then drops its own tables and
calls `geocode.Layered` / `geocode.ParseAddress` instead of its copies, keeping only what is its
own (its importer, its corrections). Its version of core must include this plan's phase 1 first.

## Decided questions (2026-11-10)

1. **The reverse-lookup limit is measured,** in phase 4, not guessed or left to each deployment.
   Until then the code uses 100 m.
2. **The new default is accepted.** `GEOCODER_URL` empty means no Nominatim before `1.0`; the
   CHANGELOG says so.
3. **The dev stack loads no CNEFE state by default.** A script fetches and loads one state on
   request.
