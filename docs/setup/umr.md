# UMR module

The UMR module surfaces the rover's connectivity in the dashboard. It polls a
Ubiquiti Mobile Router (UMR Ultra) on the rover's LAN through the router's local
portal and publishes a snapshot of WAN mode, signal, throughput, router system
state, and connected clients to the rover's NATS bus.

Read-only. The module never writes back to the router.

## Prerequisites

- A UMR Ultra (or UMR-Ultra-ROW) on the rover's LAN, reachable from the Pi.
- The router's owner password.

## Enabling

UMR is enabled through the proxy's module registry. The general mechanics are in
[no-rebuild-modules.md](no-rebuild-modules.md); the module-specific steps are:

1. Sign in to the proxy as an admin and open `/admin/modules`.
2. Register the source repository `https://github.com/waypointos/waypoint-umr`.
   The proxy fetches the latest release, verifies the cosign signature against
   the repository's own release workflow identity, and records the module.
3. Open the rover's `MODULES` tab and press **Enable on this rover** on the
   `umr` row. The config form opens before the module is enabled: enter the
   router address, the owner password, and the poll period.

```toml
host             = "https://192.168.105.1"
password         = "<owner password>"
poll_interval_s  = 5
```

Every key is optional and the values above are the module's own defaults, except
the password, which has no useful default: without it the module reaches the
router but every login is rejected.

Within ~5 seconds of the agent picking up the module, the dashboard's
`CONNECTION` tab appears for this rover (`infra.modules` reports the module
healthy).

## Changing the password later

Either the gear in the `CONNECTION` tab's top-right corner or the gear on the
rover's `MODULES` tab reopens the same form. Saving republishes the rover's
desired state; the agent rewrites `config.toml` and restarts the module, so the
new password takes effect in seconds with no reboot.

The equivalent API call is
`POST /api/admin/rovers/{roverID}/modules/umr` with
`{"version": "0.4.0", "config_toml": "..."}`. Note that `config_toml` is a
document the *agent* parses, so the keys go inside a `[modules_config.umr]`
table; the form adds that wrapper for you. Sending the flat keys above with no
table header is silently equivalent to sending no config at all.

## Release

The module lives in its own repository and ships as a signed portable service,
like every other no-rebuild module.

### 1. Repository

`waypointos/waypoint-umr` on GitHub, public. It carries `build/` (arm64 static
build plus squashfs packaging) and `.github/workflows/release.yml` (tag-triggered
release with cosign keyless signing), both already in the tree.

### 2. Monorepo access

The module builds against the monorepo SDK through `replace` directives, so both
workflows clone the monorepo next to the checkout:
`https://github.com/waypointos/waypoint.git`. The monorepo is public, so the
clone needs no deploy key and no Actions secret.

### 3. Tag a release

Tag and push:

    git tag v0.4.0
    git push origin v0.4.0

The `v*` tag triggers the workflow. Confirm the GitHub Release carries three
assets:

- `umr-0.4.0.raw`
- `umr-0.4.0.raw.cosign`
- `manifest.json`

There is no signing key to manage: cosign keyless derives the identity from the
workflow's OIDC token, which is why the workflow requests
`permissions: id-token: write`.

### 4. Register and enable

Follow [Enabling](#enabling) above. The workflow identity the proxy verifies
against is pinned on first registration and a later re-pin is refused, so moving
the repository means clearing the registry entry rather than re-registering over
it. `MODULE_TOKEN_KEY` is not involved: it is needed only for a private module
repository, see
[proxy-module-token-key.md](proxy-module-token-key.md).

## What the tab shows

- **MODE:** active WAN (wifi / lte / ethernet), wifi or LTE signal bars,
  carrier / band / technology for LTE.
- **NETWORK:** public IP, ISP, up/down throughput, latency.
- **ROUTER:** router model, firmware, uptime, router CPU/memory.
- **CLIENTS:** connected devices (name, IP, link type, link speed).

## Diagnosing N/A states

The dashboard renders each cell as either a value or `N/A — <reason>`. The
reasons:

| Reason text | Means | Fix |
|---|---|---|
| `router login failing` | Module is reaching the UMR HTTP API but the password is being rejected, including when no password is configured at all. | Re-enter the password with the gear in this tab (see [Changing the password later](#changing-the-password-later)). |
| `not on LTE` | The current WAN is wifi or ethernet, so LTE-only fields (RSRP/RSRQ/RSSI/band) are not reported. | Expected. |
| `not on wifi` | The current WAN is LTE or ethernet, so the wifi SSID/bars are not reported. | Expected. |
| `unreported` | The router itself didn't include the field in the InfoDump (firmware variation). | Open an issue with the firmware version. |

## Health semantics

The health probe answers as soon as the process is alive on the bus, so a
healthy module means "the daemon is running", not "the router is reachable". A
router that is off, unreachable, or rejecting the password leaves the module
healthy while the panel goes stale and its cells fall back to the N/A states
above. Read the table, not the health flag, when connectivity looks wrong. This
is the accepted trade-off for the SDK's readiness ordering: the module signals
ready before its first successful router login, which is what keeps a slow
router boot from turning into a systemd restart storm.

## What's not exposed

The UMR's local API caps wifi signal to a 0-4 bar scale. dBm / RSSI / bitrate /
BSSID / noise are not exposed to the owner account. The module surfaces only what
the API gives.

LTE signal *is* exposed in dBm (RSRP/RSRQ/RSSI) when LTE is the active WAN.
