#!/usr/bin/env bash
# Serves image/output/dev/images on :8000 until SIGTERM.
# QEMU smokes apply dev .swu artifacts so the new rootfs keeps its
# baked-in identity (prod images expect /data/waypoint/identity.toml
# from commissioning).
set -euo pipefail

DIR="image/output/dev/images"
[[ -d "$DIR" ]] || { echo "serve-swu.sh: missing $DIR — run 'make -C image image-dev' first" >&2; exit 1; }
command -v python3 >/dev/null || { echo "serve-swu.sh: python3 not on PATH" >&2; exit 1; }

echo "serve-swu.sh: serving $DIR on http://0.0.0.0:8000" >&2
exec python3 -m http.server --directory "$DIR" --bind 0.0.0.0 8000
