#!/usr/bin/env bash
set -euo pipefail

BINARIES_DIR="$1"

# --- Substitute PARTUUID into cmdline.txt ---
#
# rpi-firmware copied BR2_PACKAGE_RPI_FIRMWARE_CMDLINE_FILE (our
# cmdline.txt.in) to BINARIES_DIR/rpi-firmware/cmdline.txt. Render the
# {ROOTFS_PARTUUID} placeholder to partition A — the first OTA apply
# will sed-swap it to B at install time via switch-to-b.sh.
sed -i "s/{ROOTFS_PARTUUID}/AAAAAAAA-AAAA-AAAA-AAAA-AAAAAAAAAAAA/g" \
    "${BINARIES_DIR}/rpi-firmware/cmdline.txt"

# --- Run genimage to produce the .img ---
ROOTPATH_TMP="$(mktemp -d)"
trap 'rm -rf ${ROOTPATH_TMP}' EXIT

genimage \
    --rootpath "${ROOTPATH_TMP}" \
    --tmppath "${BINARIES_DIR}/genimage.tmp" \
    --inputpath "${BINARIES_DIR}" \
    --outputpath "${BINARIES_DIR}" \
    --config "${BR2_EXTERNAL_WAYPOINT_PATH}/board/raspberrypi5/genimage.cfg"

echo "[post-image] genimage produced waypoint.img"

# --- Assemble the .swu artifact (unsigned cpio) ---
#
# Trust is established by Sigstore cosign keyless signing at CI time and
# verified by the agent before this .swu is handed to swupdate. swupdate's
# own signature check is intentionally NOT enabled — cosign is the single
# trust gate.

WAYPOINT_VERSION="${WAYPOINT_VERSION:-0.5.0-dev}"
WAYPOINT_VARIANT="${WAYPOINT_VARIANT:-prod}"

cd "${BINARIES_DIR}"

# Buildroot already produced rootfs.squashfs and genimage references it
# directly for both A and B partitions, so no rename/copy is needed for
# the .swu artifact — the swupdate descriptor already expects this name.
ROOTFS_SHA256="$(sha256sum rootfs.squashfs | awk '{print $1}')"

# Substitute tokens into sw-description
sed -e "s|{WAYPOINT_VERSION}|${WAYPOINT_VERSION}|g" \
    -e "s|{WAYPOINT_VARIANT}|${WAYPOINT_VARIANT}|g" \
    -e "s|{ROOTFS_SHA256}|${ROOTFS_SHA256}|g" \
    "${BR2_EXTERNAL_WAYPOINT_PATH}/swupdate/sw-description.in" \
    > sw-description

# A/B switch scripts (postinstall) — flip cmdline.txt PARTUUID and arm
# bootcount. Mount the boot partition by PARTUUID rather than a hardcoded
# device path: /dev/mmcblk0p1 on real Pi 5, /dev/vda1 under QEMU virt.
# The CCCC PARTUUID is pinned in genimage.cfg.
cat > switch-to-a.sh <<'EOF'
#!/bin/sh
BOOT_DEV="/dev/disk/by-partuuid/cccccccc-cccc-cccc-cccc-cccccccccccc"
mount "$BOOT_DEV" /boot
sed -i 's|root=PARTUUID=[A-Fa-f0-9-]*|root=PARTUUID=AAAAAAAA-AAAA-AAAA-AAAA-AAAAAAAAAAAA|' /boot/cmdline.txt
mkdir -p /data/waypoint
echo "3" > /data/waypoint/bootcount   # 3 attempts before revert
sync
umount /boot
EOF
cat > switch-to-b.sh <<'EOF'
#!/bin/sh
BOOT_DEV="/dev/disk/by-partuuid/cccccccc-cccc-cccc-cccc-cccccccccccc"
mount "$BOOT_DEV" /boot
sed -i 's|root=PARTUUID=[A-Fa-f0-9-]*|root=PARTUUID=BBBBBBBB-BBBB-BBBB-BBBB-BBBBBBBBBBBB|' /boot/cmdline.txt
mkdir -p /data/waypoint
echo "3" > /data/waypoint/bootcount
sync
umount /boot
EOF
chmod +x switch-to-a.sh switch-to-b.sh

SWU="waypoint-${WAYPOINT_VARIANT}-${WAYPOINT_VERSION}.swu"
printf '%s\n' sw-description rootfs.squashfs switch-to-a.sh switch-to-b.sh \
    | cpio -ov -H crc > "${SWU}"

echo "[post-image] produced ${SWU} (rootfs sha256 ${ROOTFS_SHA256:0:8}…)"
echo "[post-image] sign with: cosign sign-blob --bundle ${SWU}.cosign ${SWU}"
