# Setup: no-rebuild module system

Operator-facing reference for the no-rebuild module pipeline.

## Prerequisites for a module repo

There is **no signing key to manage**: modules are signed with cosign keyless
(Sigstore), which derives identity from the GitHub Actions OIDC token. A module
repo only needs to:

- Use the template's `.github/workflows/release.yml` (it requests
  `permissions: id-token: write` and runs `cosign sign-blob`).
- Tag releases `v*` (e.g. `v0.1.0`). The tag triggers the release: CI builds
  `<id>-<version>.raw`, signs it to `<id>-<version>.raw.cosign`, and publishes
  both plus `manifest.json` as release assets.

## Registering a module

1. Sign in to the proxy as an admin.
2. Navigate to `/admin/modules` (or call `POST /api/admin/modules` with
   `{ "source_repo_url": "..." }`).
3. In **REGISTER**, paste the module repo URL. The module id is taken from the
   release manifest's `name`, so it always matches the `<id>-<version>.raw`
   asset naming.
4. Click **Register**. The proxy will:
   - Fetch the latest GitHub Release: `manifest.json`, `<id>-<version>.raw`,
     `<id>-<version>.raw.cosign`
   - Verify the cosign keyless signature; the Fulcio certificate's identity
     (SAN) must be the repo's own release workflow,
     `https://github.com/<owner>/<repo>/.github/workflows/release.yml@refs/tags/v…`,
     issued by GitHub Actions. Abort with a clear error if it doesn't match.
   - Store the artifact and record the module + version in the registry. The
     pinned workflow identity is recorded on first registration (TOFU).

### Artifact storage

Verified artifacts are stored in the proxy's S3-compatible bucket (the same
Railway bucket as the basemap tiles) under `modules/<id>/<version>.raw`,
auto-detected from the Railway-injected `AWS_S3_BUCKET_NAME`. Without a bucket
the proxy falls back to container disk, which on Railway is **ephemeral**: a
redeploy deletes the blobs while Postgres keeps the version rows, and every
rover fetch of those versions then fails until a newer version is ingested.
`WAYPOINT_PROXY_MODULE_STORAGE_DIR` forces a disk path (dev).

## Enabling a module on a rover

1. Navigate to the rover's settings → MODULES tab (future iteration; in v1
   this is operated via `POST /api/admin/rovers/{roverID}/modules/{moduleID}`
   with `{ "version": "0.1.0", "config_toml": "..." }`).
2. The proxy publishes `waypoint.<rover-id>.modules.desired`. The rover's
   reconciler picks it up within seconds, fetches the `.raw`, attaches it
   via `portablectl`, and starts the unit.

## Updating

Register a new version (proxy webhook auto-ingest is a future iteration; in
v1, paste the same repo URL again to ingest the latest GitHub Release).
Change the rover's desired version. The reconciler stops the old, attaches
the new, starts the new. If the new version's health probe fails repeatedly,
the new version is left stopped; roll back by re-selecting the previous
version.

## Verifying on the rover

- `journalctl -u waypoint-module-<id> -f` on the rover host.
- The dashboard's per-rover view shows the module's tab once the agent
  reports it healthy on `waypoint.<rover-id>.infra.modules`.

## Module template

The starter at `tools/module-template/` is the canonical skeleton. Copy it
into a new repo, edit `module.toml`, push a `v*` tag, and the release pipeline
builds the `.raw` and cosign-signs it (keyless; nothing to configure beyond
the workflow's `id-token: write` permission, which the template already sets).

## Interactive panels

A module's `[ui.static]` panel can send commands, not just render telemetry.
The host passes the panel a context with both `subscribe` and `publish`:

    mount(container, ctx) {
      // read: live module telemetry
      const off = ctx.subscribe(`waypoint.${ctx.roverId}.module.<id>.stats`, onBytes);
      // write: a command to the module's own backend
      ctx.publish(`waypoint.${ctx.roverId}.module.<id>.command`, payloadBytes);
    }

`publish` is scoped to the module's own subtree: the host rejects any subject
that is not under `waypoint.<rover>.module.<your-id>.`. A panel cannot publish
to another module or to a platform subject.

Interactive modules use a publish-command, subscribe-to-result pattern (the
same optimistic model the drive console uses): the panel publishes an action to
`module.<id>.command`, the backend acts, and the panel observes the outcome on a
telemetry subject the backend publishes (for example `module.<id>.stats` or a
module-specific state subject). This works both locally and over the proxy.
Browser-to-module request/reply is not provided: over the proxy a request to a
rover-resident responder is undeliverable, so model interactions as
command-plus-observed-result instead.

This replaces the older pattern of a module serving its own `[ui.proxy]` web app
for interactivity. `[ui.proxy]` is still supported, but new control modules
should use a standard `[ui.static]` panel so the UI lives in the dashboard tab
and works over the proxy.

## Privileged capabilities: `requires`

A manifest declares capabilities in two lists with different trust meaning:

- `provides`: outbound capabilities, where the module feeds a platform rail.
  Example: `provides = ["uplink"]` lets the agent mirror the module's
  `module.<id>.uplink` onto `telemetry.uplink`. Low privilege.
- `requires`: privileged platform access the module needs, where the agent
  brokers the module INTO a platform or core subject. High privilege, so a
  module that declares a `requires` capability only attaches when it is
  admin-registered through the signed-module trust path.

The first privileged capability is `servo-control`:

    requires = ["servo-control"]

With it declared, the agent runs a broker that bridges the module's sandboxed
servo subjects to core's generic servo surface:

- the module publishes `ServoControl` to `module.<id>.servo.cmd`; the agent
  republishes it to core's `cmd.servo`;
- the module requests `module.<id>.servo.read`; the agent proxies it to core's
  `rpc.servo_read` and returns the `ServoState`.

The module never holds `cmd.servo` or `rpc.servo_read` permissions itself; the
broker is the only thing that crosses into core's subjects, and it refuses any
op targeting the drive wheels (servo ids 7-10). Declare only the module-sandbox
servo subjects in `[permissions]`:

    [permissions]
    publish = [
      "waypoint.*.module.<id>.servo.cmd",
      "waypoint.*.module.<id>.servo.read",
    ]

Component leaves (see below) need not be declared explicitly: declaring
`[component]` auto-grants them.

## Standard component APIs: `[component]`

A manifest may declare one standard component class so the module speaks a
uniform arm/sensor/base API that any consumer can decode without reading the
manifest:

    [component]
    class = "arm"          # arm | sensor | base
    state_rate_hz = 20      # SDK publish rate for <class>.state

- `class` is validated against the known set (`arm`, `sensor`, `base`). One
  component per module in v1.
- `state_rate_hz` must be positive and at most 100; it defaults to 10 when
  omitted.

**Auto-granted leaves.** Declaring `[component]` grants the module's minted NATS
user the standard sandbox leaves for its class, alongside whatever
`[permissions]` declares. Sensor is read-only (no command leaf):

| Class | Granted publish | Granted subscribe |
|---|---|---|
| `arm` | `waypoint.*.module.<id>.arm.state` | `waypoint.*.module.<id>.arm.cmd` |
| `sensor` | `waypoint.*.module.<id>.sensor.state` | (none) |
| `base` | `waypoint.*.module.<id>.base.state` | `waypoint.*.module.<id>.base.cmd` |

The message shapes are in `protocol/messages/components.proto`. Consumers decode
the `.state` stream and publish `.cmd` over the existing module-subject paths,
so the operator side needs no permission change. The standard API is a floor,
not a ceiling: a module keeps its own private subjects (calibration, richer
commands, panel feeds) under the same sandbox.

**Discovery.** The agent's `ModuleSnapshot` (`waypoint.<rover>.infra.modules`)
carries each module's component class and state rate, so the dashboard and
future consumers discover components without reading manifests.

Authoring a module against these APIs is covered in `module-sdk.md`.

## Offline signature verification (trusted root)

Production images bake a Sigstore TUF snapshot at
`/usr/share/waypoint/sigstore/trusted_root.json`. The agent loads it via
`modverify.NewFromFile` so module signature verification works without network
access.

The file is **pinned (committed) in the repo** at the overlay path below, so
image builds are reproducible and need no cosign at build time. Regenerate it
only when Sigstore rotates its roots (rare), then reflash, by pulling the
public-good root from TUF:

```sh
cosign initialize   # caches the public-good TUF root under ~/.sigstore/root/
cp ~/.sigstore/root/targets/trusted_root.json \
    image/external/board/raspberrypi5/rootfs-overlay/usr/share/waypoint/sigstore/trusted_root.json
```

Do NOT use `cosign trusted-root create` without `--fulcio/--rekor/--ctfe/--tsa`
flags: with no flags it emits an empty root (just the mediaType header), and every
signature verification then fails at runtime with "not enough verified log entries
from transparency log". `trusted-root create` is for assembling a *private* Sigstore
instance's root, not for pinning the public-good one.

Buildroot copies the file into the rootfs via `BR2_ROOTFS_OVERLAY`
(see `waypoint_prod_defconfig`). If the file is ever absent the build still
succeeds, but offline verification then fails at runtime until the image is
refreshed. Dev images skip cosign verification (`WAYPOINT_SKIP_VERIFY=1`), so
the file is not required for local iteration.

## Diagnosing the on-rover attach path

The agent attaches a module's `.raw` with `portablectl` and then reads the
module's manifest and static dashboard assets from the attached image. These
systemd mechanics can only be exercised on a Linux host, not the dev Mac. To
validate (or debug) them on the rover, run the diagnostic:

```bash
sudo tools/module-attach-check.sh                       # synthetic image
sudo tools/module-attach-check.sh --image <id>-<ver>.raw --port <port> --start
```

It builds a Waypoint-shaped portable, runs the agent's exact attach invocation,
reports where the image's `module.toml` and `dashboard/` are actually readable,
and prints a verdict on any agent change required. Run it after a failed
drop-in or a module tab that never appears.
