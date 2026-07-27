#!/bin/sh
# waypoint-boot-modules-sync
#
# Runs every boot, before waypoint-agent.service. Mounts the FAT32 boot
# partition read-only and synchronises /data/waypoint/modules.toml with
# whatever the operator dropped alongside identity.toml.
#
# Semantics:
#   - No modules.toml on the boot partition → leave /data alone (the agent
#     tolerates the file being absent as "no modules enabled", and the
#     operator may have configured this rover via another path).
#   - modules.toml present + content matches /data → no-op.
#   - modules.toml present + content differs (or /data file absent) →
#     copy boot-partition file to /data with sha256 logged.
#
# Hash-based comparison (not mtime) because FAT32 mtimes drift when the
# SD card is mounted on macOS, which would otherwise cause spurious
# re-syncs on every boot.
#
# IMPORTANT: when a future dashboard panel writes /data/waypoint/modules.toml
# directly (via NATS RPC), this script will silently overwrite it on the
# next boot if the boot-partition file is stale. The rule until that
# panel exists: FAT32 is the source of truth. Document this on the
# operator workflow.

set -u

BOOT_DEV=/dev/disk/by-partuuid/cccccccc-cccc-cccc-cccc-cccccccccccc
MOUNT_POINT=/run/waypoint-boot-modules
SRC_REL=modules.toml
DST=/data/waypoint/modules.toml

log() { echo "[boot-modules-sync] $*" > /dev/kmsg 2>/dev/null || echo "[boot-modules-sync] $*"; }

mkdir -p /data/waypoint
mkdir -p "${MOUNT_POINT}"

cleanup() {
    umount "${MOUNT_POINT}" 2>/dev/null || true
    rmdir "${MOUNT_POINT}" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

if ! mount -t vfat -o ro "${BOOT_DEV}" "${MOUNT_POINT}" 2>/dev/null; then
    log "could not mount ${BOOT_DEV}; leaving ${DST} as-is"
    exit 0
fi

SRC="${MOUNT_POINT}/${SRC_REL}"
if [ ! -f "${SRC}" ]; then
    log "no ${SRC_REL} on boot partition; leaving ${DST} as-is"
    exit 0
fi

SRC_HASH=$(sha256sum "${SRC}" | awk '{print $1}')
DST_HASH=""
if [ -f "${DST}" ]; then
    DST_HASH=$(sha256sum "${DST}" | awk '{print $1}')
fi

if [ "${SRC_HASH}" = "${DST_HASH}" ]; then
    log "modules.toml already in sync (sha256=$(echo "${SRC_HASH}" | cut -c1-12))"
    exit 0
fi

if ! install -m 0600 "${SRC}" "${DST}"; then
    log "install of ${DST} failed"
    exit 1
fi
log "synced modules.toml from boot partition (sha256=$(echo "${SRC_HASH}" | cut -c1-12))"
