#!/bin/sh
# waypoint-bootcount-tick
# Runs once at boot. If /data/waypoint/bootcount is missing, this is a
# steady-state boot — exit. Otherwise decrement; on exhaust, swap
# cmdline.txt's PARTUUID back to the previous partition and reboot.

set -eu
COUNT=/data/waypoint/bootcount
[ -f "$COUNT" ] || exit 0

n=$(cat "$COUNT")
n=$((n - 1))

if [ "$n" -le 0 ]; then
    # Revert: swap A↔B in /boot/cmdline.txt and reboot.
    mount /dev/mmcblk0p1 /boot 2>/dev/null || true
    sed -i \
      -e 's|root=PARTUUID=AAAAAAAA-AAAA-AAAA-AAAA-AAAAAAAAAAAA|__WP_TMP__|' \
      -e 's|root=PARTUUID=BBBBBBBB-BBBB-BBBB-BBBB-BBBBBBBBBBBB|root=PARTUUID=AAAAAAAA-AAAA-AAAA-AAAA-AAAAAAAAAAAA|' \
      -e 's|__WP_TMP__|root=PARTUUID=BBBBBBBB-BBBB-BBBB-BBBB-BBBBBBBBBBBB|' \
      /boot/cmdline.txt
    rm -f "$COUNT"
    sync
    umount /boot
    echo "[bootcount-tick] exhausted — reverting partition" > /dev/kmsg
    reboot -f
fi

echo "$n" > "$COUNT"
sync
