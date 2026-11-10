#!/usr/bin/env bash
# Fetches a state's CNEFE (IBGE, Censo 2022) and loads it into the development database, for the
# geocoder's address list (docs/geocoding-plan.md). The dev stack loads none by default: run this
# for the states you work with. The file is kept in backups/cnefe/ (gitignored) and fetched once.
#
#   scripts/load_cnefe.sh DF        # 20 MB
#   scripts/load_cnefe.sh MG        # 540 MB; a few GB of memory while it loads
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
UF="$(echo "${1:?usage: $0 <UF>}" | tr '[:lower:]' '[:upper:]')"

declare -A CODES=([RO]=11 [AC]=12 [AM]=13 [RR]=14 [PA]=15 [AP]=16 [TO]=17 [MA]=21 [PI]=22 [CE]=23 [RN]=24
  [PB]=25 [PE]=26 [AL]=27 [SE]=28 [BA]=29 [MG]=31 [ES]=32 [RJ]=33 [SP]=35 [PR]=41 [SC]=42 [RS]=43 [MS]=50
  [MT]=51 [GO]=52 [DF]=53)
code="${CODES[$UF]:-}"
[[ -n "$code" ]] || { echo "unknown state $UF" >&2; exit 1; }

file="${code}_${UF}.zip"
dir="$ROOT_DIR/backups/cnefe"
url="https://ftp.ibge.gov.br/Cadastro_Nacional_de_Enderecos_para_Fins_Estatisticos/Censo_Demografico_2022/Arquivos_CNEFE/CSV/UF/$file"
mkdir -p "$dir"
if [[ ! -s "$dir/$file" ]]; then
  echo "Fetching $url ..."
  curl -fSL --progress-bar -o "$dir/$file.part" "$url"
  mv "$dir/$file.part" "$dir/$file"
fi

cd "$ROOT_DIR"
go run ./cmd/load-cnefe -file "$dir/$file" -commit
