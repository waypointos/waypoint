# Building Waypoint modules

**Date:** 2026-06-02
**Status:** authoring guide.
**References:** `docs/setup/no-rebuild-modules.md` (operator flow), `tools/module-template/` (canonical skeleton), `docs/setup/umr.md` (the worked example, a real shipping module).

---

## 1. What a module is

A Waypoint module is an optional, independently versioned extension that runs on
the rover alongside the agent (Go) and core (C++). Modules add capabilities
without rebuilding the OS image: connectivity telemetry, power monitoring, a
sensor reader, a custom dashboard tab.

The defining properties:

- **Out-of-tree.** A module lives in its **own GitHub repo**, not in the
  Waypoint monorepo. The reference module `waypoint-ubiquiti-mobile-router`
  (the "umr" / Connectivity module) is exactly this: a standalone repo built
  from the template, released by tag, registered on the proxy.
- **No image rebuild.** A module ships as a signed, self-contained `.raw`
  squashfs image. The agent fetches it, attaches it as a systemd portable
  service, and starts it. Installing or updating a module never reflashes the
  rover.
- **Sandboxed.** A module talks to the rest of the rover only over NATS, and
  only within its own subject subtree: `waypoint.*.module.<name>.*`. It cannot
  reach `cmd.*`, `telemetry.*`, `rpc.*`, or any other module. The agent enforces
  this when it mints the module's NATS user, and the manifest parser rejects any
  manifest that declares an out-of-sandbox subject.
- **Signed.** Modules are signed with cosign keyless (Sigstore). There is no key
  to manage. The signer identity is the module repo's own GitHub release
  workflow, pinned by the proxy on first registration (trust on first use).

If you are building a module, you are building a small Go (or other-language)
daemon, a manifest, a systemd unit, and a release pipeline, all in a new repo
copied from `tools/module-template/`.

---

## 2. The contract at a glance

Everything a module must satisfy:

| Obligation | How |
|---|---|
| Declare itself | `module.toml` manifest (section 4) |
| Stay in its sandbox | publish/subscribe only `waypoint.*.module.<name>.*` (section 5) |
| Take config and credentials from the agent | read the fixed files at `/run/waypoint/modules/<id>/` (section 6) |
| Answer health probes | reply on `waypoint.<rover>.module.<name>.health.<probe>` (section 6.4) |
| Signal readiness | `sd_notify(READY=1)` once the process is alive (section 6.3) |
| Ship as a signed `.raw` | the template Makefile plus the release CI (sections 8 and 9) |
| Be registered and enabled | proxy admin registers the repo, then enables per rover (section 10) |

The agent provides, at runtime, two files in `/run/waypoint/modules/<name>/`:

- `config.toml`: the per-rover configuration the operator entered at enable time.
- `creds.env`: a minted NATS user scoped to the module's sandbox
  (`WAYPOINT_NATS_USER`, `WAYPOINT_NATS_PASSWORD`).

It passes their paths on the command line and starts the process under systemd.

---

## 3. Repo layout

`tools/module-template/` is a minimal starting skeleton, but the authoritative
layout is the reference module, `waypoint-ubiquiti-mobile-router`. Its structure:

```
<your-module-repo>/
  go.mod                            # the DAEMON module (built from cmd/)
  go.sum
  module.toml                       # manifest (section 4)
  README.md
  cmd/
    waypoint-module-<id>/main.go    # entrypoint
  internal/                         # daemon packages (config, poller, publisher, ...)
  protocol/                         # module-owned .proto + generated bindings (optional)
    umr.proto
    buf.yaml  buf.gen.yaml
    gen/go/   gen/ts/
  systemd/
    waypoint-module-<id>.service    # the unit (section 7)
  build/
    go.mod                          # SEPARATE module: just the manifest.json generator
    Makefile                        # builds the .raw (section 8)
    gen-os-release.sh               # portable os-release
    gen-extension-release.sh        # extension-release marker
    manifest/manifest.go            # emits manifest.json for the release
  dashboard/                        # static UI bundle, built with Vite (section 7.2)
    package.json  src/  vite.config.ts
  vendor/                           # vendored deps for reproducible CI builds
  .github/workflows/release.yml     # cosign-keyless release on v* tags (section 9)
```

Note the **two Go modules**: the repo root `go.mod` is the daemon (entrypoint at
`cmd/waypoint-module-<id>/`), and `build/go.mod` is a small separate module whose
only job is the `manifest` tool that emits the release's `manifest.json`. They
are split so the daemon's dependency graph stays clean. The reference module also
**vendors** its dependencies (`vendor/`) so CI builds are reproducible and need no
network at build time.

> The in-repo template at `tools/module-template/` mirrors this layout exactly
> (same `cmd/` + `internal/` + `build/` split, same flag-bearing unit, same
> `build/Makefile` and release workflow), so copying it gives you the reference
> structure. The only intentional difference is that the template does not vendor
> its dependencies; add `vendor/` if you want fully offline CI builds.

When you start a new module, rename every occurrence of `example`/`umr` to your
module id: the `module.toml` fields, `cmd/waypoint-module-<id>/`,
`systemd/waypoint-module-<id>.service`, the `MODULE_ID` in `build/Makefile`, and
the release-asset names in `.github/workflows/release.yml`.

---

## 4. The manifest: `module.toml`

The manifest is the single source of truth for what the module is and what it is
allowed to do. It is baked into the `.raw` at
`/usr/share/waypoint/modules/<name>/module.toml`. The agent parses it with
`ParseManifest` in `agent/internal/modules/manifest.go`; that file is the
authority on every rule below.

A complete, annotated manifest:

```toml
name        = "example"                          # module id, see rules below
label       = "Example"                          # human label shown in the UI
version     = "0.1.0"                             # module code version (semver)
api_version = "1"                                 # informational for now; set "1"
language    = "go"                                # informational
entrypoint  = "waypoint-module-example"           # binary name in /usr/bin

[permissions]
publish   = ["waypoint.*.module.example.stats"]              # subjects the module may publish
subscribe = ["waypoint.*.module.example.health.ready"]       # incl. the health subject (see below)

[health]
probe       = "ready"                             # health subject suffix
interval_s  = 5                                   # how often the agent probes
timeout_s   = 2                                   # per-probe deadline

[ui.static]                                       # OR [ui.proxy], not both
tab_id = "m-example"                              # dashboard tab id, must start m-
bundle = "/dashboard/panel.js"                    # path inside the .raw
```

### 4.1 Field rules (enforced by the parser)

- **`name`** is required and must match `^[a-z][a-z0-9-]{0,63}$` (lowercase,
  starts with a letter, hyphens allowed, up to 64 chars). This id is the
  namespace for every subject, the runtime directory, and the minted NATS user.
- **`entrypoint`** is required. It is the binary name the agent expects at
  `/usr/bin/<entrypoint>`. Convention: `waypoint-module-<name>`.
- **`[permissions].publish`** and **`[permissions].subscribe`** must each be
  inside the module's own sandbox. Every entry has to start with
  `waypoint.*.module.<name>.` and have something after that prefix. The literal
  `*` is the rover-id wildcard; the agent substitutes the real rover id when it
  mints the NATS user. A subject like `cmd.drive` or another module's subtree is
  rejected at parse time, and the module will not load.
- **`[health].probe`** names the health subject suffix. With `probe = "ready"`,
  the agent probes `waypoint.<rover>.module.<name>.health.ready` and expects a
  prompt reply. `interval_s` and `timeout_s` tune the cadence and deadline. The
  module receives the probe by subscribing to that subject, so the health
  subject must appear under `[permissions].subscribe` (otherwise the broker ACL
  denies the responder's subscription and the module always reads as unhealthy).
- **`api_version`** is intended to pin the module-system contract, but the
  parser currently only stores it (like `language`) and does not validate it, so
  a mismatch will not fail the load today. Set `"1"`.

### 4.2 The UI block

Exactly one of `[ui.static]` or `[ui.proxy]` (declaring both is an error). You
may also omit the UI entirely (a headless telemetry module). In every case the
`tab_id` **must start with `m-`** (reserved prefix for module tabs).

- **`[ui.static]`** serves a prebuilt JS bundle baked into the `.raw`. Set
  `tab_id` and `bundle` (the path inside the image, for example
  `/dashboard/panel.js`). The agent reads the bundle from the attached image and
  the dashboard renders it as a tab. Use this for a panel that consumes the
  module's NATS telemetry.

  ```toml
  [ui.static]
  tab_id = "m-example"
  bundle = "/dashboard/panel.js"
  ```

- **`[ui.proxy]`** exposes an HTTP server the module itself runs. Set `tab_id`,
  `port`, and `lan_only` (which **must** be `true` in v1). The agent proxies the
  tab to that local port.

  ```toml
  [ui.proxy]
  tab_id   = "m-example"
  port     = 8080
  lan_only = true
  ```

### 4.3 Hardware access

If the module needs device or system access, declare it in `[hardware]`. The
agent translates this into a systemd drop-in (`DeviceAllow`, `BindReadOnlyPaths`,
`BindPaths`, `AmbientCapabilities`). Everything is allow-listed; anything outside
the lists is rejected at parse time.

```toml
[hardware]
devices      = ["/dev/i2c-1"]                 # device nodes
read_only    = ["/sys/class/hwmon"]           # read-only bind mounts
read_write   = ["/var/lib/waypoint-module-example"]  # module-private state
capabilities = ["NET_ADMIN"]                  # linux capabilities
```

Allow-lists (from `agent/internal/modules/manifest.go`):

- **devices:** prefixes `/dev/i2c-`, `/dev/spidev`, `/dev/ttyUSB`, `/dev/ttyACM`,
  `/dev/video`, `/dev/gpiochip`. **`/dev/ttyAMA*` is deliberately excluded.**
  `ttyAMA0` is the ESP32 servo byte pipe owned by core and is safety-critical;
  modules never get raw access to it. Drive-affecting actions go through NATS.
  Module serial access is USB only.
- **read_only:** prefixes `/sys/class/hwmon`, `/sys/class/gpio`,
  `/sys/class/thermal`.
- **read_write:** prefix `/var/lib/waypoint-module-` (module-private state; the
  path must include the module name).
- **capabilities:** `NET_ADMIN`, `SYS_NICE`.

### 4.4 Capabilities a module can provide

`provides` lets a module feed a platform-level signal while staying sandboxed.
The only known capability today is `uplink`:

```toml
provides = ["uplink"]
```

When a module declares `provides = ["uplink"]`, the agent mirrors that module's
`module.<name>.uplink` subject onto the platform's `telemetry.uplink`, which
feeds the dashboard's Signal indicator. The module stays inside its sandbox; the
agent does the bridging. Declaring any capability not in the known set is a parse
error.

---

## 5. Subjects and NATS conventions

A module's entire interface to the rover is NATS, confined to
`waypoint.<rover-id>.module.<name>.*`. Conventions used by shipping modules:

| Subject | Direction | Purpose |
|---|---|---|
| `waypoint.<rover>.module.<name>.stats` | module publishes | periodic telemetry snapshot (protobuf) |
| `waypoint.<rover>.module.<name>.health.<probe>` | module replies | health probe request/reply, body `"ok"` |
| `waypoint.<rover>.module.<name>.uplink` | module publishes | connectivity signal, only if `provides=["uplink"]` |

Rules:

- Use the **literal `*`** for the rover id in `module.toml`. In code, build the
  concrete subject from the `--rover` flag the agent passes you.
- Publish on subjects you declared under `[permissions].publish`. Subscribe only
  to subjects under `[permissions].subscribe`. The minted NATS user's ACL matches
  the manifest, so anything undeclared is denied by the broker at runtime.
- Telemetry should be protobuf. A module that defines its own messages owns its
  own `.proto` and generated bindings (see section 6.5). It does not edit the
  monorepo's `protocol/`.

---

## 6. The module binary contract

The daemon must cooperate with the agent's supervision model. The reference is
`tools/module-template/cmd/waypoint-module-example/main.go` (a runnable skeleton)
and the umr module's `cmd/waypoint-module-umr/main.go` (a full implementation
with config, auth, and a poll loop).

### 6.1 What the agent provides, and what it does not

This is easy to get wrong, so be precise about the boundary. Before starting a
module the agent does exactly two things:

1. Writes two files to the **fixed path** `/run/waypoint/modules/<id>/` (tmpfs,
   regenerated each agent start, creds mode `0600`):
   - `config.toml` the per-rover config the operator entered at enable time
   - `creds.env` the minted NATS user (`WAYPOINT_NATS_USER`, `WAYPOINT_NATS_PASSWORD`)
2. Runs `systemctl start waypoint-module-<id>.service`.

That is all. The agent does **not** inject any `ExecStart` flags, does not set
`--config`/`--creds`/`--rover`, does not export a config/creds env var, and does
not set a NATS URL. The only drop-in it ever writes is the hardware one
(`30-hardware.conf`, section 4.3).

So how does the daemon learn where its files are and which rover it is on? From
**its own unit**, which the module author baked into the `.raw`. The unit is
responsible for wiring the fixed paths into the process. The reference module's
unit does it with explicit flags pointing at those fixed paths and a rover id
read from `core.env` (see section 7):

```
ExecStart=/usr/bin/waypoint-module-umr \
  --config /run/waypoint/modules/umr/config.toml \
  --creds  /run/waypoint/modules/umr/creds.env \
  --rover  ${WAYPOINT_ROVER_ID}
```

A module that prefers no flags could instead read the two well-known fixed paths
directly. The template's `main.go` accepts both: the `--config`/`--creds`/
`--rover` flags its unit passes, and `WAYPOINT_MODULE_CONFIG` /
`WAYPOINT_MODULE_CREDS` / `WAYPOINT_NATS_URL` env fallbacks. Either way, the
contract is: the agent puts the files at `/run/waypoint/modules/<id>/` and starts
the unit; the unit and daemon agree on how to find them.

For the bus, the module connects on its own and defaults to the local NATS bus
(both the template and umr default to `nats://127.0.0.1:4222`, overridable via
`WAYPOINT_NATS_URL`). Authenticate with the `creds.env` user.

### 6.2 Reading config and credentials

`config.toml` carries whatever the operator entered at enable time. Define your
own schema and parse it (the umr module uses `github.com/BurntSushi/toml`):

```toml
# example config.toml, schema is module-defined
host            = "https://192.168.105.1"
password        = "<secret>"
poll_interval_s = 5
```

`creds.env` is `KEY=value` lines. Load the NATS user and authenticate with it:

```go
// loadCredsEnv reads WAYPOINT_NATS_USER / WAYPOINT_NATS_PASSWORD from creds.env.
func loadCredsEnv(path string) (user, pass string, err error) {
    f, err := os.Open(path)
    if err != nil {
        return "", "", err
    }
    defer f.Close()
    sc := bufio.NewScanner(f)
    for sc.Scan() {
        line := strings.TrimSpace(sc.Text())
        if line == "" || strings.HasPrefix(line, "#") {
            continue
        }
        k, v, ok := strings.Cut(line, "=")
        if !ok {
            continue
        }
        switch k {
        case "WAYPOINT_NATS_USER":
            user = v
        case "WAYPOINT_NATS_PASSWORD":
            pass = v
        }
    }
    return user, pass, sc.Err()
}
```

Then connect, authenticating with those credentials and reconnecting forever
(the bus or the agent can restart under you):

```go
nc, err := nats.Connect(natsURL,
    nats.UserInfo(user, pass),
    nats.MaxReconnects(-1),
    nats.ReconnectWait(2*time.Second),
)
```

### 6.3 Signaling readiness

The unit is `Type=notify`. Call `sd_notify(READY=1)` **as soon as the process is
alive and connected to NATS**, not after slow external work completes:

```go
import "github.com/coreos/go-systemd/v22/daemon"
// ...
daemon.SdNotify(false, daemon.SdNotifyReady)
```

Why this matters: systemd's default `TimeoutStartSec` is 90s. If your module
blocks readiness on something slow (a router login, an LTE attach, a device that
takes a minute to warm up), systemd will kill it mid-startup and you get a
restart storm. Signal ready early; gate **health** (not readiness) on the slow
dependency, so the agent keeps the module flagged unhealthy until the real work
succeeds, without restarting it.

### 6.4 Answering health probes

Subscribe to your health subject and reply immediately. The agent probes every
`interval_s` and marks the module unhealthy after repeated misses:

```go
subject := fmt.Sprintf("waypoint.%s.module.example.health.ready", roverID)
nc.Subscribe(subject, func(m *nats.Msg) { m.Respond([]byte("ok")) })
```

If you want "healthy" to mean "the external thing is actually working," only
register this responder (or only reply `"ok"`) once your last poll succeeded.

### 6.5 Graceful shutdown

Handle `SIGINT`/`SIGTERM` and drain. systemd sends `SIGTERM` on stop and update:

```go
ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
defer cancel()
defer nc.Drain()
```

### 6.6 Owning a protobuf message (optional)

A module that publishes structured telemetry should define its own `.proto`,
generate Go (and TypeScript for the dashboard) bindings inside the module repo,
and marshal/publish on `module.<name>.stats`. Keep it in the module repo under
`protocol/`; never edit the monorepo's `protocol/`. The dashboard panel imports
the generated TS to decode.

---

## 7. The systemd unit

`systemd/waypoint-module-<id>.service` (baked into the image at
`/usr/lib/systemd/system/`). The reference module's unit:

```ini
[Unit]
Description=Waypoint module: umr (Ubiquiti Mobile Router connectivity)
After=waypoint-agent.service
Requires=waypoint-agent.service

[Service]
Type=notify
# WAYPOINT_ROVER_ID comes from the agent's core.env (dev: /etc/waypoint,
# hardware: /data/waypoint). The agent (re)populates /run/waypoint/modules/<id>/
# before starting the unit, so config.toml and creds.env exist at exec time.
Environment=WAYPOINT_ROVER_ID=
EnvironmentFile=-/etc/waypoint/core.env
EnvironmentFile=-/data/waypoint/core.env
ExecStart=/usr/bin/waypoint-module-umr \
  --config /run/waypoint/modules/umr/config.toml \
  --creds  /run/waypoint/modules/umr/creds.env \
  --rover  ${WAYPOINT_ROVER_ID}
Restart=always
RestartSec=2s
NoNewPrivileges=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

The agent attaches the image as a **portable service** and instantiates the
concrete unit `waypoint-module-<id>.service` (the full invocation is
`portablectl attach --runtime --profile=trusted --copy=symlink ...`). The
`trusted` profile and `--runtime` let the unit read its agent-written creds and
config from `/run` and mean the minimal rover rootfs does not need to satisfy the
heavier
default sandbox. Modules are cosign-verified, which is what makes the reduced
isolation acceptable.

Note how the wiring actually works, since it is easy to get wrong:

- **The unit hardcodes the flag paths.** `--config` and `--creds` point at the
  fixed `/run/waypoint/modules/<id>/` paths the agent guarantees to populate
  before it starts the unit. The agent does not rewrite `ExecStart`.
- **The rover id comes from `core.env`,** sourced with `EnvironmentFile=-...`
  (the leading `-` makes a missing file non-fatal, so the same unit works in dev
  at `/etc/waypoint` and on hardware at `/data/waypoint`). `--rover
  ${WAYPOINT_ROVER_ID}` expands from that file.
- **`PrivateTmp=true`** is why the `.raw` must ship `tmp/` and `var/tmp/` in its
  rootfs skeleton (section 8).

---

## 8. Build and package

`build/Makefile` turns the source into a signed-ready `.raw`. The relevant
mechanics:

`build/Makefile` is written to run **from the repo root** (`make -f build/Makefile
raw`); its recipe paths are root-relative, so `make -C build` will not work. The
reference module's:

```makefile
# Run from the repo ROOT: `make raw` or `make -f build/Makefile raw`.
MODULE_ID := umr
VERSION   ?= $(shell git describe --tags --always 2>/dev/null | sed 's/^v//' || echo 0.0.0-dev)
OUT       := dist/$(MODULE_ID)-$(VERSION).raw

bin/waypoint-module-$(MODULE_ID):
	mkdir -p bin
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" \
	  -o bin/waypoint-module-$(MODULE_ID) ./cmd/waypoint-module-$(MODULE_ID)

# The dashboard panel.js must already be built (CI builds it before `raw`).
$(OUT): bin/waypoint-module-$(MODULE_ID)
	test -f dashboard/dist/panel.js || { echo "build the panel first: (cd dashboard && pnpm build)"; exit 1; }
	mkdir -p dist staging/usr/bin staging/usr/share/waypoint/modules/$(MODULE_ID) \
	         staging/usr/lib/extension-release.d staging/usr/lib/systemd/system staging/dashboard
	# rootfs skeleton: systemd binds host /dev,/proc,/sys (+ /tmp,/var/tmp for
	# PrivateTmp) onto these mount points for the portable namespace. Without
	# them the unit fails at step NAMESPACE (status 226).
	mkdir -p staging/dev staging/proc staging/sys staging/run staging/tmp \
	         staging/var/tmp staging/etc staging/root
	touch staging/etc/machine-id staging/etc/resolv.conf
	cp bin/waypoint-module-$(MODULE_ID) staging/usr/bin/
	cp module.toml staging/usr/share/waypoint/modules/$(MODULE_ID)/
	cp systemd/waypoint-module-$(MODULE_ID).service staging/usr/lib/systemd/system/
	cp dashboard/dist/panel.js staging/dashboard/panel.js
	bash build/gen-os-release.sh $(MODULE_ID) $(VERSION) > staging/usr/lib/os-release
	bash build/gen-extension-release.sh $(MODULE_ID) $(VERSION) \
	     > staging/usr/lib/extension-release.d/extension-release.$(MODULE_ID)
	mksquashfs staging $(OUT) -comp zstd -no-progress -all-root
	rm -rf staging
```

Key points:

- **Target is `arm64`** (the Pi 5). Always `GOOS=linux GOARCH=arm64 CGO_ENABLED=0`.
- **The daemon builds from `./cmd/waypoint-module-<id>`** using the root `go.mod`.
- **The dashboard panel must be built first.** The `raw` target hard-fails if
  `dashboard/dist/panel.js` is missing; CI runs `pnpm build` in `dashboard/`
  before `make raw`. The panel is copied to `/dashboard/panel.js` in the image,
  matching the `bundle` path in `module.toml`.
- **`VERSION` strips a leading `v`** so the tag `v0.1.0` yields `umr-0.1.0.raw`.
- **Image layout inside the squashfs:**
  - `/usr/bin/waypoint-module-<id>` the binary
  - `/usr/share/waypoint/modules/<id>/module.toml` the manifest
  - `/usr/lib/systemd/system/waypoint-module-<id>.service` the unit
  - `/usr/lib/os-release` with `PORTABLE_PREFIXES=waypoint-module`
  - `/usr/lib/extension-release.d/extension-release.<id>` the portable marker
  - `/dashboard/panel.js` the static UI bundle, if any
  - the empty skeleton dirs (`dev`, `proc`, `sys`, `run`, `tmp`, `var/tmp`,
    `etc`, `root`) plus empty `etc/machine-id` and `etc/resolv.conf`. These are
    mandatory: the portable namespace binds host paths onto them. Omitting them
    makes the unit fail to start with a NAMESPACE/226 error.
- **`os-release`** declares `PORTABLE_PREFIXES=waypoint-module` so `portablectl`
  recognizes the unit prefix. `gen-os-release.sh` and `gen-extension-release.sh`
  emit these from the module id and version.
- **The build/release artifact is named `<id>-<version>.raw`** (the Makefile
  `OUT` and the release asset). Note this is only the *published* name. The
  agent downloads it and stores it locally as `<id>.raw`, with the version in
  the parent directory: `<root>/modules/<id>/<version>/<id>.raw`. That rename is
  deliberate: `portablectl` derives the portable name from the file stem, so the
  stem must equal the module id exactly for `attach`/`detach <id>` and the
  `<id>`-keyed mount paths to line up. Do not assume the attached file keeps the
  `<id>-<version>` form.

Build locally on any host with `mksquashfs` (`squashfs-tools`):

```sh
make -f build/Makefile raw VERSION=0.1.0
```

The squashfs assembly and the systemd attach can only be fully exercised on
Linux. See section 11 for the on-rover diagnostic.

---

## 9. Signing and release (cosign keyless)

Modules are signed with **cosign keyless**: no key to store, no secret to
configure. The signer identity is the module repo's own release workflow,
derived from a short-lived GitHub Actions OIDC token, and the proxy pins that
identity on first registration.

`.github/workflows/release.yml` (copy verbatim, change only the asset names):

The reference module's workflow (note the dashboard build step and that the
manifest generator runs from its own `build/` module):

```yaml
name: Release
on:
  push:
    tags: ['v*']

permissions:
  contents: write
  id-token: write   # cosign keyless OIDC; the proxy pins this workflow's SAN

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version-file: go.mod }
      - uses: pnpm/action-setup@v4
        with: { version: 9 }
      - uses: actions/setup-node@v4
        with:
          node-version: 22
          cache: pnpm
          cache-dependency-path: dashboard/pnpm-lock.yaml
      - run: sudo apt-get update && sudo apt-get install -y squashfs-tools
      - uses: sigstore/cosign-installer@v3
      - name: Set VERSION
        run: echo "VERSION=${GITHUB_REF_NAME#v}" >> "$GITHUB_ENV"
      - name: Build dashboard panel
        run: cd dashboard && pnpm install --frozen-lockfile && pnpm build
      - name: Build .raw
        run: make -f build/Makefile raw VERSION="${VERSION}"
      - name: Sign .raw (cosign keyless)
        run: |
          cosign sign-blob --yes --new-bundle-format \
            --bundle "dist/umr-${VERSION}.raw.cosign" \
            "dist/umr-${VERSION}.raw"
      - name: Manifest
        run: |
          SHA="$(sha256sum "dist/umr-${VERSION}.raw" | cut -d' ' -f1)"
          (cd build && go run ./manifest -toml ../module.toml \
            -version "${VERSION}" -sha256 "${SHA}") > dist/manifest.json
      - uses: softprops/action-gh-release@v2
        with:
          files: |
            dist/umr-${{ env.VERSION }}.raw
            dist/umr-${{ env.VERSION }}.raw.cosign
            dist/manifest.json
```

`id-token: write` lets the job mint a GitHub OIDC token; cosign exchanges it at
Fulcio for a signing cert whose SAN is this workflow's identity, which the proxy
pins at registration time. No secrets. A module without a UI drops the pnpm/node
setup and the dashboard build step; a module whose daemon and manifest generator
share one `go.mod` runs `go run ./build/manifest` directly instead of `cd build`.

A `v*` tag triggers a release that publishes three assets:

- `<id>-<version>.raw` the image
- `<id>-<version>.raw.cosign` the cosign bundle (new bundle format)
- `manifest.json` a JSON sidecar (the manifest fields plus `version` and
  `sha256`), generated by `build/manifest`

The proxy verifies the cosign signature offline against a pinned Sigstore
trusted root. It requires the Fulcio certificate SAN to be the repo's own
`.../.github/workflows/release.yml@refs/tags/v...`, issued by GitHub Actions. A
mismatch aborts registration. The rover does the same offline verification using
the trusted root baked at `/usr/share/waypoint/sigstore/trusted_root.json`.

To cut a release:

```sh
git tag v0.1.0 && git push --tags
```

---

## 10. Registration, enablement, and reconciliation

Once a release exists, an operator brings it onto rovers. This is operator work,
documented in `docs/setup/no-rebuild-modules.md`; the short version:

1. **Register** (proxy admin, `/admin/modules`): paste the module repo URL and
   id. The proxy fetches the latest release, verifies the signature, pins the
   signer identity (trust on first use), and records the module in the registry.
2. **Enable per rover**: set the desired version and the per-rover
   `config_toml`. The proxy publishes `waypoint.<rover>.modules.desired`.
3. **Reconcile** (on the rover, automatic): the agent's reconciler picks up the
   desired set within seconds, fetches the `.raw` (verifying the sha256 and the
   signature), attaches it with `portablectl`, writes `config.toml` and
   `creds.env`, mints the NATS user, and starts the unit.

Updates are the same loop: register a new version, change the rover's desired
version, and the reconciler stops the old unit and starts the new. If the new
version's health probe fails repeatedly, it is left stopped so the operator can
roll back.

A module can also be installed locally (offline, from a `.raw` on disk) without
the proxy; local desired-state wins over proxy desired-state on conflict. That
path is for development and air-gapped rovers.

---

## 11. Testing and diagnostics

- **Unit-test the daemon** like any Go program. Keep the NATS publish loop, the
  config parsing, and the external-API mapping in separate packages so they test
  in isolation (the umr module splits `config`, `poller`, and `publisher`).
- **Build the `.raw` locally** with `make -f build/Makefile raw` on any host with
  `squashfs-tools`.
- **Exercise the attach path on the rover.** systemd portable mechanics only run
  on Linux, not the dev Mac. Use the monorepo diagnostic:

  ```sh
  sudo tools/module-attach-check.sh                                   # synthetic image
  sudo tools/module-attach-check.sh --image <id>-<ver>.raw --start    # your image
  ```

  It runs the agent's exact `portablectl attach` invocation, reports where the
  image's `module.toml` and `dashboard/` are actually readable, and prints a
  verdict on any agent change required. Run it when a drop-in fails or a module
  tab never appears.
- **On the rover, follow the logs:** `journalctl -u waypoint-module-<id> -f`.
- The dashboard shows the module's tab once the agent reports it healthy on
  `waypoint.<rover>.infra.modules`.

For local iteration, dev images skip cosign verification
(`WAYPOINT_SKIP_VERIFY=1`), so you can attach an unsigned `.raw` without a
trusted root.

---

## 12. Hard rules and gotchas

- **Stay in the sandbox.** Only `waypoint.*.module.<name>.*`. The manifest parser
  rejects anything else, and the broker denies undeclared subjects at runtime.
- **Never touch `/dev/ttyAMA*`.** That UART is core's safety-critical servo pipe.
  It is excluded from the hardware allow-list on purpose. Module serial access is
  USB only (`ttyUSB`/`ttyACM`).
- **Signal `READY=1` early; gate health, not readiness, on slow dependencies.**
  Otherwise systemd's 90s start timeout turns a slow boot into a restart storm.
- **Ship the rootfs skeleton in the `.raw`.** The empty `dev/proc/sys/...` dirs
  and `etc/machine-id`, `etc/resolv.conf` are required for the portable
  namespace. Missing them is the classic NAMESPACE/226 attach failure.
- **Release asset is `<id>-<version>.raw`, but the attached file is `<id>.raw`.**
  The agent stores the download as `<id>.raw` (version in the parent dir) because
  `portablectl` derives the portable name from the file stem, which must equal
  the module id exactly. Build/publish `<id>-<version>.raw`; do not expect that
  stem to survive to attach time.
- **`tab_id` must start with `m-`.** Reserved prefix for module tabs.
- **One UI mode.** `[ui.static]` and `[ui.proxy]` are mutually exclusive; proxy
  UIs must be `lan_only = true` in v1.
- **Do not edit the monorepo `protocol/`.** A module owns its own `.proto` in its
  own repo.
- **Build for `arm64`.** The rover is a Pi 5.
- **The signer identity is your release workflow.** Keep `.github/workflows/release.yml`
  intact with `id-token: write`; the proxy pins it on first registration, so
  changing the workflow path later breaks signature verification.

---

## 13. Worked example: the umr (Connectivity) module

`waypoint-ubiquiti-mobile-router` is the canonical shipping module. It is a Go
daemon that polls a Ubiquiti Mobile Router on the rover LAN through the router's
local portal and publishes a connectivity snapshot. It demonstrates every part
of this guide:

- **Manifest** with `name = "umr"`, `label = "Connectivity"`, a single publish
  subject `waypoint.*.module.umr.stats`, a `ready` health probe, and a
  `[ui.static]` tab `m-umr`.
- **Config** (`host`, `password`, `poll_interval_s`) supplied per rover at enable
  time, read from the agent-written `config.toml`.
- **Auth** from `creds.env`, connecting to the bus as the minted `module-umr`
  user.
- **Early readiness, gated health.** It signals `READY=1` once NATS is connected,
  then retries the router login in a loop, because the router can take a minute
  to attach LTE on first boot. Health stays unhealthy until a poll succeeds.
- **Own protobuf** (`UmrStats`) published on `module.umr.stats`, decoded by a
  static dashboard panel.
- **N/A as a first-class state.** Fields the router does not report render as a
  muted `N/A` with a one-line reason, never a sentinel zero.

The operator-facing behavior of this module is documented in `docs/setup/umr.md`.
Read that alongside this guide to see the system end to end: a module repo, its
release, its registration, and what the operator sees.
```
