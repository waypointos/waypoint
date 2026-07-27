# image/

## Prerequisites (macOS dev)

- Docker Desktop or OrbStack (15 GB free disk space minimum)
- ~30 minutes for a cold build; ~3 minutes for incremental rebuilds (ccache hot)
- `brew install qemu expect` for the QEMU boot/OTA/rollback smokes

## First build

```bash
cd image
make docker-image      # one-time, builds the Buildroot Docker image
make image-prod        # produces output/prod/images/waypoint.img + waypoint-prod-<ver>.swu
```

## Dev build

The dev variant is the iteration image: SSH, debug tools, mock motors, and
verification disabled, built locally and flashed straight to a card. It is
never produced by CI and never published as a release (see "Dev vs CI" below).

```bash
cd image
make image-dev         # produces output/dev/images/waypoint.img + waypoint-dev-<ver>.swu
```

`make image-dev` runs every prerequisite itself (vendors the agent, rebuilds
and embeds the dashboard, builds the Buildroot Docker image on first run,
creates the build volume), so it is the only command you need. Because the
vendor and embed steps run on the host before the container starts, the host
needs Go (for `go mod vendor`) and pnpm (for `pnpm build`) installed. The
first run is a cold Buildroot build (~30 min); later runs are incremental
(ccache hot, roughly 3 min plus the forced application rebuild).

### The iteration loop

`image-dev` exists to make "edit source, rebuild, reflash" work with zero
version bookkeeping. Before each build it force-`dirclean`s the in-repo
packages (`waypoint-agent`, `waypoint-core`, `waypoint-bootcheck`,
`waypoint-firstboot`) plus a set of upstream packages whose kconfig and
kernel-fragment changes Buildroot's stamp cache would otherwise miss
(`linux`, `rpi-firmware`, `gstreamer1` and its plugin packages, `libv4l`).
A source edit therefore always lands in the next image without moving any
VERSION.

This is the deliberate difference from `image-prod`, which keeps stable
package versions and relies on ccache for reproducibility, so a source change
that needs to ship is gated behind an explicit `WAYPOINT_VERSION` bump. Do
not use the dev loop to produce a shipping image.

### Knobs

| Env var | Default | Effect |
|---|---|---|
| `WAYPOINT_VERSION` | `0.0.0-dev` | Stamped into `/etc/waypoint/image.toml` and the `.swu` filename (`waypoint-dev-<ver>.swu`). Release builds get theirs from the `image-v*` tag; local dev builds rarely need to set it. |
| `WAYPOINT_VARIANT` | `dev` (set by the target) | Recorded in `image.toml`; selects the dev rootfs overlay and skip-verify. Do not override. |
| `WAYPOINT_EXTRA_OVERLAY` | empty | Absolute path to an extra rootfs overlay layered last, for one-off local files. |
| `JOBS` | host CPU count | Parallel build jobs, e.g. `make image-dev JOBS=4`. |

```bash
make image-dev WAYPOINT_VERSION=0.6.0-dev
```

### Full from-scratch rebuild

When a stale stamp escapes the dirclean list (rare, usually a Buildroot
config option that neither package list covers), wipe and rebuild:

```bash
make image-dev-clean   # removes output/dev (and the macOS build volume), then image-dev
```

### What the dev variant changes vs prod

The dev defconfig is the prod defconfig plus the deltas below. Each has its
own section later in this doc:

- **SSH (dropbear)** with a baked `authorized_keys`. See "SSH access (dev image only)".
- **Verification disabled:** dev images write `WAYPOINT_SKIP_VERIFY=1` to `/etc/waypoint/agent.env`, so unsigned local `.swu` files apply. With the bypass active the agent also skips the `.cosign` sidecar download, so a dev `.swu` served on its own (no bundle next to it) applies cleanly. The agent logs loudly while this bypass is active. See "Signing".
- **Baked identity and mock motors:** falls back to `/etc/waypoint/identity.toml.dev` and bakes `--servo-mock` when no real `identity.toml` is present. See "Identity behavior on dev images".
- **Layered overlays:** `rootfs-overlay` then `rootfs-overlay-dev` (later wins). Keep dev-only files under `rootfs-overlay-dev/` so they never leak into prod.

### Output location (macOS vs Linux)

On Linux (CI, native dev) Buildroot writes straight to `image/output/dev`. On
macOS the heavy build runs inside a named Docker volume (to dodge a virtiofs
flush race), and only the final artifacts are copied out to
`image/output/dev/images`. Either way, the artifacts you flash live in
`output/dev/images/`.

### Dev vs CI

There is no dev path in CI or the Releases feature. `image-build.yml` builds
and signs `prod` only, on `image-v*` tags. Dev images are local-build,
unsigned (skip-verify), and flashed directly to the card or applied with
`swupdate` over SSH. The `/releases` dashboard "dev" channel therefore has
nothing to catalog until a signed dev `.swu` is published, which is out of
scope today.

## Cutting a release

Versions are not picked by hand. `make release-image` (repo root, needs
`brew install git-cliff`) derives the next `image-v*` version from the
conventional commits since the last image tag (`fix` bumps patch, `feat`
bumps minor, a breaking change bumps major), prepends the new section to
the root `CHANGELOG.md`, and prints the exact commit/tag/push commands.
Only commits touching `agent/`, `core/`, `dashboard/`, `protocol/`, or
`image/` count toward the image line; proxy-only work never bumps it.

```bash
make release-image     # updates CHANGELOG.md, prints the commands
# review the CHANGELOG.md diff, then run the printed git commands
```

Pushing the tag triggers `image-build.yml`: a clean Buildroot build,
cosign signing, and a GitHub Release carrying `waypoint.img`,
`waypoint-prod-<ver>.swu`, their `.cosign` bundles, and `SHA256SUMS`.
Expect 30 to 45 minutes. `make release-proxy` is the same flow for the
proxy line (`proxy/CHANGELOG.md`, `proxy-v*` tags).

Commits from before the changelog was introduced are not conventional;
the `0.1.0` entry summarizes that era by hand and history stays as it is.

## Buildroot menuconfig (when you need to tweak the rootfs)

```bash
cd image && make menuconfig
```

This drops you into a TTY-driven kernel-style menu. Save and exit; the
config diff lives in `external/configs/waypoint_prod_defconfig`.

## Signing: Sigstore cosign keyless

There are no long-lived signing keys to manage. CI signs each `.swu` (and
the `.img`) with `cosign sign-blob` using a short-lived Fulcio certificate
bound to the workflow's OIDC identity. The resulting Sigstore bundle is
uploaded as `<artifact>.cosign` alongside the artifact in the GitHub
Release. Rekor records every signature.

On the rover, `agent/internal/verify` verifies the bundle before invoking
`/usr/bin/swupdate`. The verifier pins:

- cert Subject regex: `^https://github\.com/waypointos/waypoint/\.github/workflows/image-build\.yml@refs/tags/image-v.+$`
- OIDC issuer: `https://token.actions.githubusercontent.com`
- Rekor inclusion proof: required.

If any pin fails the apply is rejected and swupdate is never invoked.

### Dev / QEMU smoke

QEMU smokes use the dev-variant image, which sets `WAYPOINT_SKIP_VERIFY=1`
in `/etc/waypoint/agent.env`. The agent logs loudly when this bypass is
active. Production images do not ship this file.

To smoke-verify a real signed artifact locally without writing code:

```bash
cosign verify-blob \
  --bundle waypoint-prod-0.5.0.swu.cosign \
  --certificate-identity-regexp '^https://github\.com/waypointos/waypoint/\.github/workflows/image-build\.yml@refs/tags/image-v.+$' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  waypoint-prod-0.5.0.swu
```

### Rotation

Automatic. Each workflow run uses a fresh Fulcio cert. Nothing to rotate,
no bridge image to ship.

### Air-gap caveat

The first verification on a rover fetches the Sigstore TUF root from
`tuf-repo-cdn.sigstore.dev`. After that it caches locally. Fully
air-gapped deployments would need an embedded TUF root, deferred for a
future iteration. (Module verification already loads a baked
`trusted_root.json`; image `.swu` verification still fetches the root live.)

## SSH access (dev image only)

Prod images ship without `dropbear`/`openssh`; the only ways onto a
prod rover are HDMI + USB keyboard or the agent's NATS surface. Dev
images run dropbear and bake an `authorized_keys` from
`image/external/board/raspberrypi5/rootfs-overlay-dev/root/.ssh/authorized_keys`.

That path is **gitignored**; each developer drops their own pubkey
there before running `make image-dev`. If the file is absent, the build
still succeeds but dropbear has no keys to accept, so SSH won't work.

```sh
ssh-keygen -t ed25519 -f ~/.ssh/waypoint_rover -C "waypoint-rover-dev"
mkdir -p image/external/board/raspberrypi5/rootfs-overlay-dev/root/.ssh
cp ~/.ssh/waypoint_rover.pub \
   image/external/board/raspberrypi5/rootfs-overlay-dev/root/.ssh/authorized_keys
make -C image image-dev
# flash, then:
ssh -i ~/.ssh/waypoint_rover root@<rover-ip>
```

`post-build.sh` enforces the strict perms dropbear requires (`/root/.ssh`
0700, `authorized_keys` 0600) since git wouldn't preserve them anyway.

### Identity behavior on dev images

`waypoint-firstboot` looks for `identity.toml` on the FAT32 boot
partition first; if absent, it falls back to the baked
`/etc/waypoint/identity.toml.dev` (only present in dev images). Either
way it writes `/data/waypoint/identity.toml` and seeds
`/data/waypoint/core.env` with the matching `rover.id`. So:

- **Dev image, no commissioning:** boots as `qemu-smoke-rover`, NATS
  local-only, never appears on the proxy. Useful for QEMU smokes and
  early hardware bring-up.
- **Dev image, commissioned `identity.toml` on boot partition:** boots
  as the real rover, registers with the proxy, **appears online**.
  Keeps SSH + debug tools. Recommended for on-bench debugging.
- **Prod image, commissioned `identity.toml`:** same as above but no
  SSH. The production deployment path.

When a real commissioning file is loaded, firstboot also clears
`WAYPOINT_CORE_ARGS` in `/data/waypoint/core.env` so the dev image's
baked `--servo-mock` doesn't leak onto real motor hardware.

## A/B and bootcount

Pi 5 boots via the firmware's normal flow (no U-Boot bootcount; the same
A/B-failover semantics with one fewer moving part). A/B selection is encoded in `cmdline.txt`'s
`root=PARTUUID=<a|b>`. swupdate writes the new rootfs to the inactive
partition, swaps `cmdline.txt`, sets a bootcount file at
`/data/waypoint/bootcount`, then reboots.

The `waypoint-bootcheck` systemd unit runs once the agent reports healthy
(by writing `/run/waypoint/healthy`) and clears `bootcount`. If the agent
never reports healthy, `waypoint-bootcount-tick` decrements `bootcount` on
each boot; when it hits zero, the tick service swaps `cmdline.txt`'s
PARTUUID back to the previous partition and reboots. See
`image/external/package/waypoint-bootcheck/`.

## QEMU smoke (Mac)

`-M virt` is used today because upstream QEMU lacks a `raspi5b` machine
type. When it lands, swap the machine type in `image/scripts/qemu-run.sh`
for full Pi-firmware fidelity.

```bash
make image-qemu-boot       # boots image, asserts agent + core active (~2 min)
make image-qemu-ota        # applies signed v0.5.1, verifies A→B swap (~3 min)
make image-qemu-rollback   # broken update → 3 failed boots → auto-revert (~5–8 min)
```

The rollback smoke is the highest-value QEMU stage; auto-revert is the
safety property that's hardest to validate without hardware.

## Pi 5 hardware smoke

1. Flash `output/prod/images/waypoint-prod-<ver>.img` to a microSD with
   `dd` or Raspberry Pi Imager. Card needs ≥16 GB.
2. Drop `identity.toml` (downloaded from the proxy enroll wizard) at the
   root of the FAT32 boot partition, next to `config.txt`. The
   `waypoint-firstboot` oneshot copies it to `/data/waypoint/identity.toml`
   on first boot, parses `rover.id`, writes `/data/waypoint/core.env`
   with the matching ID, and orders itself `Before=waypoint-agent.service`.
   Re-runs only if `/data` is wiped; the source file on the boot
   partition is left in place for re-flash recovery.
3. Power on. Within ~20 seconds the rover should appear online in the
   proxy fleet view with the correct image version on the RoverCard.
4. From the dashboard's admin "Apply update" dialog, paste a .swu URL
   (e.g. a GitHub Release asset). Watch the rover reboot, come back on
   the other partition, and report the new version.
5. Failure smoke: ship a deliberately-broken .swu (e.g. wrong signature).
   swupdate should reject it without rebooting. Then ship one whose agent
   fails to start; after 3 reboot loops the rover should auto-revert.

## Hardware cheat sheet

Quick reference for at-the-keyboard Pi debugging. Partition layout (pinned):
boot=`cccccccc-...`, rootfs-A=`aaaa...`, rootfs-B=`bbbb...`, /data=`dddd...`.
On Pi 5 those map to `/dev/mmcblk0p{1,2,3,4}` in order.

### Which partition are we booted on?

`/etc/waypoint/image.toml`'s `partition` field is set at build time and
always says A; it does not reflect the actual running partition. Use
the kernel cmdline instead:

```sh
grep -o 'PARTUUID=[^ ]*' /proc/cmdline
# aaaa... = running rootfs-A   bbbb... = running rootfs-B
findmnt -no SOURCE /
# /dev/mmcblk0p2 = A   /dev/mmcblk0p3 = B
```

### Version + agent + core state

```sh
grep version /etc/waypoint/image.toml
systemctl is-active waypoint-agent waypoint-core
journalctl -u waypoint-agent -n 50 --no-pager
journalctl -u waypoint-core  -n 50 --no-pager
```

### Read cmdline.txt (what the firmware will use next boot)

```sh
mkdir -p /tmp/boot
mount /dev/disk/by-partuuid/cccccccc-cccc-cccc-cccc-cccccccccccc /tmp/boot
cat /tmp/boot/cmdline.txt        # look for root=PARTUUID=AAAA... or BBBB...
umount /tmp/boot
```

### Bootcount (probationary boot counter)

```sh
cat /data/waypoint/bootcount 2>/dev/null || echo "no probation in progress"
# absent = stable boot, last update committed
# 3,2,1  = probationary boot N attempts remaining
# 0      = next boot should trigger revert
```

### Apply an OTA manually (bypass agent RPC)

```sh
# Booted on A → write copy-2 (rootfs-B). Booted on B → use copy-1.
SWU_URL=https://github.com/waypointos/waypoint/releases/download/image-vX.Y.Z/waypoint-prod-X.Y.Z.swu
wget -O /tmp/u.swu "$SWU_URL"
swupdate -e prod,copy-2 -i /tmp/u.swu

# Verify post-install before rebooting
mount /dev/disk/by-partuuid/cccccccc-cccc-cccc-cccc-cccccccccccc /tmp/boot
grep -o 'PARTUUID=[^ ]*' /tmp/boot/cmdline.txt   # should now be bbbb...
umount /tmp/boot
cat /data/waypoint/bootcount                     # should be 3
```

### Useful state files

```sh
/data/waypoint/identity.toml      # rover ID + bootstrap token (prod path)
/etc/waypoint/identity.toml.dev   # dev variant baked-in identity
/data/waypoint/core.env           # WAYPOINT_ROVER_ID for waypoint-core (prod)
/etc/waypoint/core.env            # dev variant override
/etc/waypoint/agent.env           # WAYPOINT_SKIP_VERIFY=1 etc (dev only)
/etc/waypoint/image.toml          # build-time metadata, NOT runtime partition
/run/waypoint/nats.sock           # agent ↔ core IPC
/run/waypoint/healthy             # agent's "I'm up" signal for bootcheck
/data/waypoint/bootcount          # probationary counter, absent when stable
```

### Network reachability

```sh
ip -br addr                         # confirm eth0 / wlan0 has an IP
ping -c 2 1.1.1.1                   # confirm internet
nslookup ghcr.io                    # confirm DNS for cosign + release fetch
```

### What QEMU can NOT verify (so this is the Pi-only delta)

- Pi firmware reads `cmdline.txt` from the boot partition at every boot
  → the A/B kernel switch only takes effect on real hardware.
- Auto-revert after 3 failed probationary boots needs the firmware-honoured
  cmdline path, so only validates on real hardware.
- USB camera enumeration, real motor-controller serial bus, GPIO.
