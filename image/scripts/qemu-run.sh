#!/usr/bin/env bash
# Boots image/output/<variant>/images/waypoint.img under
# qemu-system-aarch64 -M virt. Pass --dev for the dev variant; --headless
# for non-interactive automation.
#
# Networking: user-mode NAT. Host:8081→guest:8080 (HTTP), 14222→4222 (NATS).
# Host is reachable from guest at 10.0.2.2.
#
# We use -M virt because upstream QEMU lacks a `raspi5b` machine type yet.
# When `-M raspi5b` lands, swap to it here for full Pi-firmware fidelity.

set -euo pipefail

VARIANT="prod"
HEADLESS=0
while [[ $# -gt 0 ]]; do
    case "$1" in
        --dev)        VARIANT="dev"; shift ;;
        --headless)   HEADLESS=1; shift ;;
        *)            echo "unknown arg: $1"; exit 1 ;;
    esac
done

OUT="image/output/${VARIANT}/images"
[[ -f "$OUT/Image" ]]        || { echo "missing $OUT/Image — run make image-${VARIANT}"; exit 1; }
[[ -f "$OUT/waypoint.img" ]] || { echo "missing $OUT/waypoint.img"; exit 1; }

# QEMU 11+ treats `-nographic` as "video off AND serial→stdio AND
# monitor→stdio multiplexed". Adding a second `-serial stdio` (or even
# `-serial mon:stdio`) on top conflicts with the implicit one. Use
# `-display none` instead so we can route serial explicitly.
ARGS=(
    -M virt
    -cpu cortex-a76
    -m 4G
    -smp 4
    -kernel "$OUT/Image"
    -append "root=PARTUUID=AAAAAAAA-AAAA-AAAA-AAAA-AAAAAAAAAAAA rootfstype=squashfs ro rootwait console=ttyAMA0"
    -drive  "file=$OUT/waypoint.img,format=raw,if=none,id=disk0"
    -device "virtio-blk-device,drive=disk0"
    -netdev "user,id=net0,hostfwd=tcp::8081-:8080,hostfwd=tcp::14222-:4222"
    -device "virtio-net-device,netdev=net0"
    -display none
)

# mon:stdio multiplexes the QEMU monitor and the guest serial on stdio:
# normal typing goes to the guest, Ctrl-A then c switches to the monitor
# (where `quit` exits), Ctrl-A then x exits QEMU directly. Suitable for
# both interactive use and `expect`-driven smokes.
exec qemu-system-aarch64 "${ARGS[@]}" -serial mon:stdio
