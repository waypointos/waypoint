#!/usr/bin/env bash
#
# Runs after `make target` (all packages installed into the rootfs at
# TARGET_DIR). The PARTUUID-substitution for cmdline.txt happens in
# post-image.sh instead — by then rpi-firmware has copied cmdline.txt.in
# into BINARIES_DIR/cmdline.txt, which is what genimage actually packs
# into the boot.vfat partition. Writing to TARGET_DIR/boot/cmdline.txt
# would land in the squashfs rootfs and be shadowed by the real boot
# partition at runtime — wasted bytes.
set -euo pipefail

TARGET_DIR="$1"

# Optional second overlay (e.g. build-broken-swu.sh injects a failing
# waypoint-agent here to exercise the auto-revert smoke).
if [ -n "${WAYPOINT_EXTRA_OVERLAY:-}" ] && [ -d "${WAYPOINT_EXTRA_OVERLAY}" ]; then
    echo "[post-build] applying extra overlay from ${WAYPOINT_EXTRA_OVERLAY}"
    cp -a "${WAYPOINT_EXTRA_OVERLAY}/." "${TARGET_DIR}/"
fi

# Bake the build's image version into the rootfs.
VERSION="${WAYPOINT_VERSION:-0.5.0-dev}"
VARIANT="${WAYPOINT_VARIANT:-prod}"
PARTITION="${WAYPOINT_PARTITION:-A}"

mkdir -p "${TARGET_DIR}/etc/waypoint"
cat > "${TARGET_DIR}/etc/waypoint/image.toml" <<EOF
[image]
version   = "${VERSION}"
variant   = "${VARIANT}"
partition = "${PARTITION}"
built_at  = "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
EOF

# Enable systemd-time-wait-sync so time-sync.target only reaches "active"
# after timesyncd has actually synchronized the clock with an NTP server.
# Without this, time-sync.target completes immediately and any unit with
# After=time-sync.target (notably waypoint-agent.service) starts while the
# clock is still at the image build timestamp, triggering TLS handshake
# failures against the proxy until sync eventually lands. Pi 5 has no RTC,
# so this is the every-boot path.
mkdir -p "${TARGET_DIR}/etc/systemd/system/sysinit.target.wants"
ln -sf /usr/lib/systemd/system/systemd-time-wait-sync.service \
    "${TARGET_DIR}/etc/systemd/system/sysinit.target.wants/systemd-time-wait-sync.service"

# Regenerate /lib/modules/<kver>/modules.dep + aliases. Buildroot's
# automatic target-finalize depmod uses $(LINUX_VERSION_PROBED), which
# returns the upstream version (e.g. "6.12.61") and ignores Pi Foundation's
# LOCALVERSION suffix ("-v8-16k"). The mismatch means the auto-depmod
# writes an empty index into the actual on-disk dir name — modprobe can't
# resolve any module by name, and udev's hotplug auto-load never fires.
# Symptom: USB UVC cameras plug in, kernel logs the descriptor, but no
# /dev/video* node appears and uvcvideo never binds.
for mod_dir in "${TARGET_DIR}/lib/modules/"*; do
    [ -d "${mod_dir}/kernel" ] || continue
    kver=$(basename "${mod_dir}")
    echo "[post-build] regenerating module index for ${kver}"
    depmod -a -b "${TARGET_DIR}" "${kver}"
done

# Remove swupdate's auto-start units. The agent invokes `swupdate -i <url>`
# directly from its apply handler (one-shot), so the long-running daemon
# is unused. Worse, swupdate.socket binds :8080 and collides with the
# agent's dashboard. BR2_PACKAGE_SWUPDATE_WEBSERVER=n in the defconfig
# should already prevent this in modern Buildroot, but Buildroot's swupdate
# package has historically installed the .socket unit unconditionally —
# remove it defensively so the collision never resurfaces.
rm -f "${TARGET_DIR}/usr/lib/systemd/system/swupdate.socket"
rm -f "${TARGET_DIR}/usr/lib/systemd/system/swupdate.service"
rm -f "${TARGET_DIR}/etc/systemd/system/sockets.target.wants/swupdate.socket"
rm -f "${TARGET_DIR}/etc/systemd/system/multi-user.target.wants/swupdate.service"

# Make sure /data mountpoint exists, then ensure exactly one fstab entry
# for it. Buildroot's TARGET_DIR is incremental across rebuilds, so we
# strip any prior /data line before appending — otherwise we accumulate
# duplicates and systemd-fstab-generator refuses with "Duplicate entry".
#
# The PARTUUID is lowercase on purpose: genimage normalizes partition
# UUIDs to lowercase when writing the GPT, blkid reads them back
# lowercase, and udev creates /dev/disk/by-partuuid/ symlinks in
# lowercase. The kernel's `root=PARTUUID=` parser is case-insensitive
# so cmdline.txt can stay uppercase, but systemd resolves PARTUUID via
# the udev symlink and that lookup IS case-sensitive.
mkdir -p "${TARGET_DIR}/data"
touch "${TARGET_DIR}/etc/fstab"
sed -i '/[[:space:]]\/data[[:space:]]/d' "${TARGET_DIR}/etc/fstab"
echo "PARTUUID=dddddddd-dddd-dddd-dddd-dddddddddddd /data ext4 defaults,noatime 0 2" \
    >> "${TARGET_DIR}/etc/fstab"

# The bulk "store" partition (module images, recordings, module-state),
# mounted inside /data/waypoint so existing config paths are untouched.
# Same lowercase-PARTUUID and dedupe rationale as /data above. nofail so a
# missing/late store device never blocks boot; waypoint-resize grows it to
# fill the card on first boot.
mkdir -p "${TARGET_DIR}/data/waypoint/store"
sed -i '/[[:space:]]\/data\/waypoint\/store[[:space:]]/d' "${TARGET_DIR}/etc/fstab"
echo "PARTUUID=eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee /data/waypoint/store ext4 defaults,noatime,nofail 0 2" \
    >> "${TARGET_DIR}/etc/fstab"

# Dev images skip cosign verification on apply_image so QEMU smokes and
# local iterations don't require Sigstore-signed artifacts. They also
# ship a baked-in identity.toml.dev which waypoint-firstboot uses as a
# fallback when no commissioning file was dropped on the boot partition
# — so QEMU smokes and on-bench debug boots come up without provisioning,
# while a real commissioned identity (dropped on FAT32) still wins.
# Production images require commissioning, which is a Phase 6 (auth/RBAC)
# deliverable. The agent's verifier treats an absent WAYPOINT_SKIP_VERIFY
# as "verify required".
if [ "${VARIANT}" = "dev" ]; then
    cat > "${TARGET_DIR}/etc/waypoint/agent.env" <<EOF
WAYPOINT_SKIP_VERIFY=1
EOF
    # Baked dev identity. waypoint-firstboot installs this to
    # /data/waypoint/identity.toml when no commissioning file is present
    # on the FAT32 boot partition. Stable rover ID so OTA smokes can
    # predict topic names. No [proxy] block by design — dev image runs
    # NATS local-only unless commissioned with a real identity.
    cat > "${TARGET_DIR}/etc/waypoint/identity.toml.dev" <<EOF
[rover]
id = "qemu-smoke-rover"
name = "QEMU Smoke Rover"

[local]
bootstrap_admin_token = "smoke-do-not-use-in-prod"
EOF
    # Fallback core env: applies only when /data/waypoint/core.env is
    # absent (e.g. before firstboot has run, or if firstboot couldn't
    # parse rover.id). Once firstboot writes /data, /data wins per the
    # service's EnvironmentFile ordering. --servo-mock swaps the real
    # ttyAMA0 motor-bus driver for an in-memory mock — without it, core
    # writes motor frames to /dev/ttyAMA0, which on QEMU virt is the
    # kernel console, garbling the serial we use for the smoke.
    cat > "${TARGET_DIR}/etc/waypoint/core.env" <<EOF
WAYPOINT_ROVER_ID=qemu-smoke-rover
WAYPOINT_CORE_ARGS=--servo-mock
EOF

    # dropbear refuses to read authorized_keys if /root/.ssh is group/world
    # writable or the key file is group/world readable, and git doesn't
    # reliably preserve mode bits on files in the rootfs-overlay-dev tree.
    # Enforce the perms here so dev SSH actually works after a fresh clone.
    if [ -f "${TARGET_DIR}/root/.ssh/authorized_keys" ]; then
        chmod 0700 "${TARGET_DIR}/root/.ssh"
        chmod 0600 "${TARGET_DIR}/root/.ssh/authorized_keys"
    fi
fi

echo "[post-build] version=${VERSION} variant=${VARIANT} partition=${PARTITION}"
