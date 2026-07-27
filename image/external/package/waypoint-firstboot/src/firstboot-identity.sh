#!/bin/sh
# waypoint-firstboot-identity
#
# Runs once on first boot (and again only if /data/waypoint/identity.toml
# is ever deleted). Provisions the writable data partition from one of
# two identity sources, in priority order:
#
#   1. /<boot-fat32>/identity.toml — operator-supplied commissioning file.
#      The production path: download from the proxy enroll wizard, drop
#      it onto the SD card alongside config.txt, power on.
#   2. /etc/waypoint/identity.toml.dev — baked dev-image fallback so
#      QEMU smokes and on-bench debug boots come up without a separate
#      commissioning step. Absent on prod images.
#
# After installing /data/waypoint/identity.toml, we parse rover.id and
# write /data/waypoint/core.env so the agent and core daemons agree on
# which rover ID to publish under without a second provisioning step.
#
# Failure modes are deliberately soft: if neither source is available,
# we log and exit 0. The agent will then fail loudly on a missing
# identity, which is the right surface for that error — we don't want
# to block boot here and prevent recovery.

set -u

# PARTUUID is pinned in genimage.cfg (boot partition) and gets a udev
# symlink at /dev/disk/by-partuuid/, so this is stable across mmcblk
# (Pi SD card) vs. vda (QEMU virt) device naming.
BOOT_DEV=/dev/disk/by-partuuid/cccccccc-cccc-cccc-cccc-cccccccccccc
MOUNT_POINT=/run/waypoint-boot
SRC_REL=identity.toml
DST=/data/waypoint/identity.toml
DEV_TEMPLATE=/etc/waypoint/identity.toml.dev
CORE_ENV=/data/waypoint/core.env

log() { echo "[firstboot-identity] $*" > /dev/kmsg 2>/dev/null || echo "[firstboot-identity] $*"; }

mkdir -p /data/waypoint
mkdir -p "${MOUNT_POINT}"

cleanup() {
    umount "${MOUNT_POINT}" 2>/dev/null || true
    rmdir "${MOUNT_POINT}" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

SOURCE=""
COMMISSIONED=0
if mount -t vfat -o ro "${BOOT_DEV}" "${MOUNT_POINT}" 2>/dev/null; then
    if [ -f "${MOUNT_POINT}/${SRC_REL}" ]; then
        SOURCE="${MOUNT_POINT}/${SRC_REL}"
        COMMISSIONED=1
        log "using commissioned identity from boot partition"
    else
        log "${SRC_REL} not found on boot partition"
    fi
else
    log "could not mount ${BOOT_DEV}; skipping boot partition lookup"
fi

if [ -z "${SOURCE}" ] && [ -f "${DEV_TEMPLATE}" ]; then
    SOURCE="${DEV_TEMPLATE}"
    log "falling back to dev template at ${DEV_TEMPLATE}"
fi

if [ -z "${SOURCE}" ]; then
    log "no identity source available; rover will wait for commissioning"
    exit 0
fi

if ! install -m 0600 "${SOURCE}" "${DST}"; then
    log "install of ${DST} failed"
    exit 1
fi
log "installed ${DST}"

# Extract rover.id from the [rover] section. Restricting to that section
# avoids false matches against other id-like keys in nested tables.
ROVER_ID=$(awk '
    /^\[rover\]/ { in_rover = 1; next }
    /^\[/        { in_rover = 0; next }
    in_rover && /^[[:space:]]*id[[:space:]]*=/ {
        sub(/^[^=]*=[[:space:]]*"/, "")
        sub(/".*$/, "")
        print
        exit
    }
' "${DST}")

if [ -z "${ROVER_ID}" ]; then
    log "could not parse rover.id from ${DST}; core.env not written"
    exit 0
fi

# Write /data/waypoint/core.env. Per waypoint-core.service's
# EnvironmentFile ordering, /data is loaded last and so wins on collision
# with the dev image's baked /etc/waypoint/core.env. When we provisioned
# from a real commissioning file (not the dev template), explicitly clear
# WAYPOINT_CORE_ARGS so a dev image deployed on real hardware doesn't
# inherit --servo-mock from the QEMU defaults.
{
    printf 'WAYPOINT_ROVER_ID=%s\n' "${ROVER_ID}"
    if [ "${COMMISSIONED}" = "1" ]; then
        printf 'WAYPOINT_CORE_ARGS=\n'
    fi
} > "${CORE_ENV}"
chmod 0644 "${CORE_ENV}"
log "wrote ${CORE_ENV} (rover-id=${ROVER_ID}, commissioned=${COMMISSIONED})"
