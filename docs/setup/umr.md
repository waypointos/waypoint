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

UMR is enabled through the proxy's module registry. See
[no-rebuild-modules.md](no-rebuild-modules.md) for the registration and
enablement flow. At enablement time, supply the configuration TOML:

```toml
host             = "https://192.168.105.1"
password         = "<owner password>"
poll_interval_s  = 5
```

Within ~5 seconds of the agent picking up the module, the dashboard's
`CONNECTION` tab appears for this rover (`infra.modules` reports the module
healthy).

## What the tab shows

- **MODE:** active WAN (wifi / lte / ethernet), wifi or LTE signal bars,
  carrier / band / technology for LTE.
- **NETWORK:** public IP, ISP, up/down throughput, latency.
- **ROUTER:** router model, firmware, uptime, router CPU/memory.
- **CLIENTS:** connected devices (name, IP, link type, link speed).

## Diagnosing N/A states

The dashboard renders each cell as either a value or `N/A: <reason>`. The reasons:

| Reason text | Means | Fix |
|---|---|---|
| `router login failing` | Module is reaching the UMR HTTP API but the password is being rejected. | Re-check the password in the module configuration; restart the agent. |
| `not on LTE` | The current WAN is wifi or ethernet, so LTE-only fields (RSRP/RSRQ/RSSI/band) are not reported. | Expected. |
| `not on wifi` | The current WAN is LTE or ethernet, so the wifi SSID/bars are not reported. | Expected. |
| `unreported` | The router itself didn't include the field in the InfoDump (firmware variation). | Open an issue with the firmware version. |

## What's not exposed

The UMR's local API caps wifi signal to a 0-4 bar scale. dBm / RSSI / bitrate /
BSSID / noise are not exposed to the owner account. The module surfaces only what
the API gives.

LTE signal *is* exposed in dBm (RSRP/RSRQ/RSSI) when LTE is the active WAN.
