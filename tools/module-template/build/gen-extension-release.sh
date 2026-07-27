#!/usr/bin/env bash
set -euo pipefail
ID="$1"
VERSION="$2"
cat <<EOF
ID=waypoint-module-${ID}
VERSION_ID=${VERSION}
EXTENSION_RELOAD_MANAGER=1
EOF
