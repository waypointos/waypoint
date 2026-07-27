#!/bin/sh
#
# Grow the trailing "store" partition to fill the whole card, then grow
# its ext4 filesystem online. One-shot: the systemd unit guards on the
# sentinel, and we only write the sentinel after a successful (or
# already-maxed) resize, so a failure leaves it un-armed to retry on the
# next boot.
#
# Buildroot 2026.02 has no growpart package, so we drive sgdisk directly:
# move the GPT backup header to the end of the now-larger device, then
# recreate the last partition to fill the free space, preserving its start
# sector, type, and PARTUUID. Preserving the PARTUUID is essential: fstab
# and the /data/waypoint/store mount resolve the device by PARTUUID.
#
# Device derivation is by PARTUUID + sysfs (no lsblk), so it works on both
# SD (mmcblk0p5) and USB (sda5) without hardcoding a device path.
set -u

STORE_PARTUUID="eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee"
STORE_TYPECODE="8300"            # GPT "Linux filesystem" (0FC63DAF-...)
SENTINEL="/data/waypoint/.store-resized"

log() { echo "[waypoint-resize] $*"; }

# Resolve the store block device from its PARTUUID symlink.
dev="$(readlink -f "/dev/disk/by-partuuid/${STORE_PARTUUID}" 2>/dev/null || true)"
if [ -z "${dev}" ] || [ ! -b "${dev}" ]; then
    log "store partition (PARTUUID=${STORE_PARTUUID}) not found; nothing to grow"
    exit 0
fi
base="$(basename "${dev}")"            # e.g. mmcblk0p5 or sda5

# Partition number, parent disk, and start sector, all from sysfs.
partnum="$(cat "/sys/class/block/${base}/partition" 2>/dev/null || true)"
start="$(cat "/sys/class/block/${base}/start" 2>/dev/null || true)"
parent="$(basename "$(readlink -f "/sys/class/block/${base}/..")" 2>/dev/null || true)"
if [ -z "${partnum}" ] || [ -z "${start}" ] || [ -z "${parent}" ] || [ ! -b "/dev/${parent}" ]; then
    log "could not derive disk/partnum/start for ${dev} (parent=${parent} num=${partnum} start=${start}); aborting"
    exit 0
fi
disk="/dev/${parent}"
log "growing ${dev} (disk=${disk} part=${partnum} start=${start})"

# Relocate the GPT backup header to the end of the (physically larger)
# device, then delete + recreate the last partition filling the free space
# (end sector 0 = largest possible), restoring its start, type, PARTUUID,
# and name. Idempotent: if there is no trailing free space (e.g. QEMU,
# where the disk equals the .img size) the recreate yields the same
# geometry and resize2fs becomes a no-op.
if ! sgdisk --move-second-header "${disk}"; then
    log "sgdisk --move-second-header failed; leaving sentinel un-armed"
    exit 1
fi
if ! sgdisk \
        --delete="${partnum}" \
        --new="${partnum}:${start}:0" \
        --typecode="${partnum}:${STORE_TYPECODE}" \
        --partition-guid="${partnum}:${STORE_PARTUUID}" \
        --change-name="${partnum}:store" \
        "${disk}"; then
    log "sgdisk recreate failed; leaving sentinel un-armed"
    exit 1
fi

# Poke the kernel to pick up the new partition size while it stays mounted
# (BLKPG via partx; a full re-read is impossible because the rootfs lives
# on the same disk and is mounted).
partx -u "${dev}" 2>/dev/null || log "partx -u returned nonzero (continuing)"

# Online-grow the ext4 to the new partition size.
if ! resize2fs "${dev}"; then
    log "resize2fs failed; leaving sentinel un-armed to retry"
    exit 1
fi

# Arm the one-shot guard only on a genuinely successful write; if /data is
# unwritable we must leave the sentinel absent so the next boot retries
# rather than falsely reporting success.
if ! mkdir -p "$(dirname "${SENTINEL}")" || ! : > "${SENTINEL}"; then
    log "store grown but could not write sentinel ${SENTINEL}; will retry next boot"
    exit 1
fi
log "store grown; sentinel written at ${SENTINEL}"
exit 0
