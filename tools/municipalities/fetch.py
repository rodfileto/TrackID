"""Write IBGE municipality boundaries as GeoJSON for cmd/load-municipalities.

The boundaries come from geobr (IPEA, MIT licence; the data is IBGE's), in
SIRGAS 2000; they are written in WGS 84 (EPSG:4326), which locations use, with
the properties the loader reads: code (7 digits), name, uf.

    uv run python fetch.py --states RJ --out municipalities.geojson
    go run ./cmd/load-municipalities -file tools/municipalities/municipalities.geojson -commit
"""
import argparse

import geobr
import pandas as pd

YEAR = 2022


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--states", default="RJ", help="comma-separated UFs, or 'all' for Brazil")
    ap.add_argument("--out", default="municipalities.geojson")
    ap.add_argument("--full", action="store_true", help="full-resolution boundaries (default: geobr's simplified)")
    args = ap.parse_args()

    states = ["all"] if args.states == "all" else [s.strip().upper() for s in args.states.split(",")]
    frames = [geobr.read_municipality(code_muni=s, year=YEAR, simplified=not args.full) for s in states]
    gdf = pd.concat(frames, ignore_index=True).to_crs(epsg=4326)

    out = gdf.assign(
        code=gdf["code_muni"].astype("int64").astype(str).str.zfill(7),
        name=gdf["name_muni"],
        uf=gdf["abbrev_state"],
    )[["code", "name", "uf", "geometry"]]
    if out["code"].duplicated().any():
        raise SystemExit("duplicate municipality codes")
    out.to_file(args.out, driver="GeoJSON")
    print(f"wrote {args.out}: {len(out)} municipalities ({', '.join(states)}), geobr year {YEAR}, "
          f"{'full' if args.full else 'simplified'} boundaries")


if __name__ == "__main__":
    main()
