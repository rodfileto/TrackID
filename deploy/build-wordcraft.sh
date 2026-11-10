#!/usr/bin/env bash
# Builds WordCraft, the report editor and PDF renderer, for the TrackID image, into ./wordcraft/,
# which compose.wordcraft.yml mounts into the API (docs/deployment.md, "Reports"). The image is
# Debian 12, so wordcraft-cli is compiled in a Debian 12 Rust container: a build from a newer
# system won't run in it (its glibc is too old). The web editor is WebAssembly, the same anywhere,
# and is taken from the checkout's dist/web (`trunk build --release` in apps/wordcraft-web).
#
#   ./build-wordcraft.sh <wordcraft checkout> [<craft-i18n checkout>]
#
# The checkout is mounted read-only. Without craft-i18n (at the commit in craft-i18n.rev) the
# editor's interface is English only; the reports it renders are the same.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"

src="$(cd "${1:?usage: $0 <wordcraft checkout> [<craft-i18n checkout>]}" && pwd)"
i18n="${2:-}"
RUST_IMAGE="rust:1.99.0-bookworm"
[[ -f "$src/Cargo.toml" ]] || { echo "$src is not a WordCraft checkout" >&2; exit 1; }
[[ -f "$src/dist/web/index.html" ]] || { echo "no $src/dist/web: run 'trunk build --release' in $src/apps/wordcraft-web first" >&2; exit 1; }

mounts=(-v "$src:/src:ro" -v trackid-wordcraft-target:/target -v trackid-wordcraft-cargo:/usr/local/cargo/registry)
envs=(-e CARGO_TARGET_DIR=/target)
if [[ -n "$i18n" ]]; then
  mounts+=(-v "$(cd "$i18n" && pwd):/i18n:ro")
  envs+=(-e CRAFT_I18N_DIR=/i18n)
fi

mkdir -p wordcraft
echo "==> wordcraft-cli, in $RUST_IMAGE (the first build takes several minutes)"
docker run --rm "${mounts[@]}" "${envs[@]}" -v "$PWD/wordcraft:/out" -w /src "$RUST_IMAGE" sh -c \
  "cargo build --release --locked -p wordcraft-cli && cp /target/release/wordcraft-cli /out/ && chown $(id -u):$(id -g) /out/wordcraft-cli"

echo "==> the web editor"
rm -rf wordcraft/web
cp -r "$src/dist/web" wordcraft/web
rm -f wordcraft/web/.trunk-lock
echo "    $(git -C "$src" rev-parse --short HEAD 2>/dev/null || echo unknown) -> $PWD/wordcraft (cli and web)"
