#!/usr/bin/env bash
set -euo pipefail
ID="$1"
VERSION="$2"
cat <<EOF
NAME="Waypoint Module: ${ID}"
ID=waypoint-module-${ID}
VERSION_ID=${VERSION}
PORTABLE_PREFIXES=waypoint-module
EOF
