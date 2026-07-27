#!/usr/bin/env bash
# Builds a dev .swu whose waypoint-agent binary exits 1 immediately. After
# 3 probationary boot attempts the bootcount-tick service must revert.
#
# We build the dev variant so the new rootfs still ships
# /etc/waypoint/identity.toml.dev — if we shipped a prod variant here, the
# baked-in identity would disappear and the rollback would fire for the
# wrong reason (agent crash from missing identity, not from broken binary).
#
# Requires image/Makefile's image-dev target to honour the
# WAYPOINT_EXTRA_OVERLAY env var (forwarded into the Docker run as
# BR2_ROOTFS_OVERLAY append).
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
VERSION=${WAYPOINT_VERSION:-0.5.2-broken}
OVERLAY="${REPO_ROOT}/image/scripts/broken-overlay"

mkdir -p "${OVERLAY}/usr/bin"
cat > "${OVERLAY}/usr/bin/waypoint-agent" <<'EOF'
#!/bin/sh
echo "[broken-agent] intentional failure for rollback smoke" >&2
exit 1
EOF
chmod +x "${OVERLAY}/usr/bin/waypoint-agent"

WAYPOINT_VERSION="${VERSION}" \
  WAYPOINT_EXTRA_OVERLAY="${OVERLAY}" \
  make -C image image-dev
