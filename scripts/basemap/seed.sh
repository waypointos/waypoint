#!/usr/bin/env bash
# Build the Waypoint basemap: world overview (z0-7) + the regions.geojson areas
# (z8-15), merged into one PMTiles file. Output is NEVER committed — upload it
# to the proxy's tile bucket (see docs/setup/basemap-bucket.md).
#
# Requires the pmtiles CLI (https://docs.protomaps.com/pmtiles/cli):
#   go install github.com/protomaps/go-pmtiles@latest   # installs as `go-pmtiles`
# SRC must point at a CURRENT planet build — the demo bucket is gone and URLs
# rotate, so confirm the latest at https://docs.protomaps.com/basemaps/downloads
# (Source Cooperative mirror or maps.protomaps.com/builds).
set -euo pipefail
cd "$(dirname "$0")"

# `go install` names the binary go-pmtiles; a manual build may be pmtiles.
PM="${PMTILES:-$(command -v pmtiles || command -v go-pmtiles || true)}"
[ -n "$PM" ] || { echo "pmtiles CLI not found — go install github.com/protomaps/go-pmtiles@latest" >&2; exit 1; }

: "${SRC:?Set SRC to a current planet pmtiles URL or path}"
OUT="${OUT:-basemap.pmtiles}"

echo "==> world overview z0-7"
"$PM" extract "$SRC" world.pmtiles --maxzoom=7

echo "==> regions z8-15 (from regions.geojson)"
"$PM" extract "$SRC" cities.pmtiles --region=regions.geojson --minzoom=8 --maxzoom=15

echo "==> merge -> $OUT"
rm -f "$OUT"
"$PM" merge world.pmtiles cities.pmtiles "$OUT"

echo "==> done: $OUT"
"$PM" show "$OUT" | head -n 20
