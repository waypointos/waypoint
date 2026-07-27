# Drill module: release and install

The drill module (lift plus auger) lives in its own repository and ships as a
signed portable service, like every other no-rebuild module. The general
registration and enablement mechanics are in
[no-rebuild-modules.md](no-rebuild-modules.md); this document covers only what
is specific to the drill, in the order it has to happen.

First powered run on the rover is a separate document,
[drill-bringup.md](drill-bringup.md).

## 1. Create the repository

Create `waypointos/waypoint-drill` on GitHub and push `main`. The repository
carries `build/` (arm64 static build plus squashfs packaging) and
`.github/workflows/release.yml` (tag-triggered release with cosign keyless
signing), both already in the tree.

## 2. Monorepo access

The module builds against the monorepo SDK through `replace` directives, so
the release workflow clones the monorepo next to the checkout:
`https://github.com/waypointos/waypoint.git`. The monorepo is public, so the
clone needs no deploy key or Actions secret.

## 3. Tag a release

Tag and push:

    git tag v0.1.0
    git push origin v0.1.0

The `v*` tag triggers the workflow. Confirm the GitHub Release carries three
assets:

- `drill-0.1.0.raw`
- `drill-0.1.0.raw.cosign`
- `manifest.json`

There is no signing key to manage: cosign keyless derives the identity from
the workflow's OIDC token, which is why the workflow requests
`permissions: id-token: write`.

## 4. Register and enable through the proxy

1. Sign in to the proxy as an admin and open `/admin/modules` (equivalently,
   `POST /api/admin/modules` with `{"source_repo_url": "..."}`).
2. Paste the repository URL and register. The proxy fetches the latest
   release, verifies the cosign signature against the repository's own
   release workflow identity, and records the module. That workflow identity
   is pinned on first registration and a later re-pin is refused, so moving
   the repository means clearing the registry entry rather than re-registering
   over it.
3. Enable it on a rover:
   `POST /api/admin/rovers/{roverID}/modules/drill` with
   `{"version": "0.1.0", "config_toml": "..."}`. The proxy publishes the
   rover's desired module state; the agent fetches the `.raw`, attaches it,
   and starts the unit within seconds.

`MODULE_TOKEN_KEY` is needed only if the module repository is private, see
[proxy-module-token-key.md](proxy-module-token-key.md). A public repository
needs nothing beyond the steps above.

## 5. Install locally instead

The same module installs without the proxy, from the rover's own dashboard.
Open the rover's MODULES panel and upload `drill-<version>.raw` together with
its `drill-<version>.raw.cosign` bundle. The install runs in two calls:

- **stage** verifies the keyless signature against the release-workflow
  identity, offline, before anything is written.
- **confirm** records the rover-side pin on first install (later installs
  must match it) and takes the optional config TOML.

Uploads are multipart with a 256 MB cap, which the drill's `.raw` is far
inside.

A locally installed module is not visible in the proxy's module registry. If
the rover is operated through the proxy, register and enable it there
instead, per section 4.

## 6. No reflash for signature verification

Production images already bake the Sigstore trusted root at
`/usr/share/waypoint/sigstore/trusted_root.json`, so verifying the module's
signature needs no network and no image change, by either path. See the
trusted-root section of [no-rebuild-modules.md](no-rebuild-modules.md).

The agent on the flashed image still has to be new enough for what the drill
declares. `module.toml` carries `requires = ["servo-control", "teleop-input"]`
and a `[component]` class, and all three are brokered agent-side: the servo
bridge, the teleop-input bridge, and the component subject auto-grant. On an
older agent the image attaches and the unit starts, but the module gets no
servo access and no grants for its component subjects. If the module reports
healthy while the drill tab shows no motor rows and the unit log reports
denied subjects, that agent is the cause and the rover needs a newer image.

## 7. Configuration skeleton

Supply this as the `config_toml` at enable time (proxy) or at confirm time
(local). Every key is optional and the values below are the module's own
defaults, so a first install can send nothing at all and set the wiring signs
during bring-up.

```toml
# Wiring signs. Confirm both on the rover before drilling: see drill-bringup.md.
lift_up_sign     = 1
auger_drill_sign = 1
# "ccw" turns opposite the drill sign, "cw" turns with it.
switch_direction = "ccw"

# Servo ids on the shared STS3215 bus.
lift_id  = 11
auger_id = 12

# Speeds in raw ticks per second. An unhomed lift always jogs at slow_jog_speed.
jog_speed      = 400
slow_jog_speed = 150
drill_speed    = 800
switch_speed   = 300

# Stall detection for the home and travel-calibration procedures.
stall_load      = 600
stall_ticks     = 10
stall_speed_eps = 20
stall_delta_eps = 8

# Switch interlock window, as a fraction of travel measured from the top.
top_band_fraction = 0.03

# Hard backstop, written to both servos at startup. Not a tuning knob.
lift_overcurrent_raw  = 500
auger_overcurrent_raw = 500

# Halt timers in milliseconds: bus read gap, and operator input staleness.
read_gap_halt_ms = 250
stale_input_ms   = 150

# Optional. Set it to make height_mm available; without it that field stays N/A.
# mm_per_tick = 0.05

# Optional. Where the calibrated travel span is persisted.
# state_path = "/var/lib/waypoint-module-drill/calibration.toml"
```

With the module enabled and healthy, continue with
[drill-bringup.md](drill-bringup.md).
