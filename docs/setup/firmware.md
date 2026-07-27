# servo-relay firmware

ESP32 firmware that bridges the Pi's UART (GPIO 14/15) to the STS3215 servo
bus on the Waveshare Bus Servo Driver HAT.

> **Note:** the shipped OS image does not use this relay path. It drives the
> servo bus over the HAT's USB serial bridge (`/dev/ttyACM0`); see the
> hardware smoke section of `docs/setup/core.md`. This firmware supports the
> alternate GPIO-UART topology (`/dev/ttyAMA0`) described below.

## macOS prerequisites

```
brew install cmake ninja dfu-util python3
cd ~/Developer
git clone -b v5.3 --recursive https://github.com/espressif/esp-idf.git
cd esp-idf && ./install.sh esp32
```

Source the IDF environment per shell. For fish:

```
source ~/Developer/esp-idf/export.fish
idf.py --version
```

A shell function alias (e.g. `get_idf`) that sources `export.fish` is
a common convenience for fish users.

## Build, test, flash

```
make firmware-test        # host-runnable parser tests
make firmware-build       # compiles via idf.py
make firmware-flash       # refuses if /dev/ttyAMA0 is held; otherwise flashes
```

## Pinout (verified)

| Signal              | ESP32 GPIO | Notes                                          |
|---------------------|------------|------------------------------------------------|
| UART0 TX → Pi RXD0  | 1          | Wired to Pi GPIO 15 via HAT header             |
| UART0 RX ← Pi TXD0  | 3          | Wired to Pi GPIO 14 via HAT header             |
| UART1 TX → SP3485   | 19         | Vendor `S_TXD = 19`                            |
| UART1 RX ← SP3485   | 18         | Vendor `S_RXD = 18`                            |
| RS485 DE/RE         | n/a        | Driven automatically by HAT's Q1 transistor    |

The HAT schematic is published on the Waveshare wiki page for the Bus Servo
Driver HAT.

## One-time Pi setup

Enable the UART and disable the login getty on `/dev/ttyAMA0`:

```
sudo raspi-config
  → 3 Interface Options → I6 Serial Port
  → "No" to login shell over serial
  → "Yes" to serial port hardware
sudo reboot
```

## Operational constraint

ESP32 UART0 is electrically tied to **both** the Pi GPIO header **and** the
HAT's USB-C programming port (Type_C2). Only one transmitter at a time.

- **Production:** Pi only. USB-C unplugged.
- **Flash / development:** USB-C only. Stop `waypoint-core` first so it
  releases `/dev/ttyAMA0`. `make firmware-flash` checks this for you.

## Hardware bring-up checklist

Run after any wiring change or firmware-behaviour change. Each step gates
the next; don't proceed on a failure.

**Step A: ESP boots and runs.** Flash, plug USB-C. Verify HAT power LED
and ~50 mA current draw. Build a debug variant with logs enabled to
`idf.py monitor` to confirm `app_main` reaches the task-spawn point.
*Gate:* no crash, no watchdog fire.

**Step B: UART loopback on the bench.** HAT off Pi. Jumper UART0 TX
(GPIO 1) ↔ UART1 RX (GPIO 18) and UART1 TX (GPIO 19) ↔ UART0 RX (GPIO 3)
externally. From a USB-serial dongle on Type_C2, send a known pattern.
*Gate:* byte-perfect round trip at 1 Mbps for ≥ 10 k bytes.

**Step C: Pi sees the relay.** HAT installed, no servos attached. Run
`waypoint-core --servo-port /dev/ttyAMA0 --rover-id pi-01`. Send a
`Ping(id=1)` → expect a clean timeout. *Gate:* core's timeout fires
correctly, no spurious bytes in core's read buffer.

**Step D: One servo, set ID.** Power 11.1 V bus, connect one STS3215.
Set ID = 1, then ping. *Gate:* ping returns a valid response frame;
`frames_ok` counter increments by 1.

**Step E: Continuous read at production rate.** With one servo, run
core's 20 Hz `readState` loop for 60 s. Read counters via the dev-only
USB CDC tap. *Gate:* `frames_ok ≈ 1200`, `bad_checksum == 0`,
`truncated == 0`. Validates auto-DE timing under sustained load.

**Step F: Four wheels chained.** Repeat E with IDs 1–4. *Gate:*
`frames_ok ≈ 4800`, error counters still 0.

**Step G: Watchdog failover regression.** Existing test from
`docs/setup/core.md` step 5, with the relay in the path. Kill the agent
→ within 500 ms, core publishes `event.fault.heartbeat_lost` and wheels
coast to a stop. *Gate:* behaviour identical to pre-relay topology.

**Step H: Soak.** Drive the rover via the dashboard for 30 minutes of
mixed motion. *Gate:* error counters remain 0, no servo dropouts visible
in dashboard telemetry.

## Checking the firmware version on a device

Production builds emit zero bytes on UART0 (no boot banner, no logs).
To check which firmware is flashed, attach over USB-C in dev mode:

```
cd firmware/servo-relay
idf.py monitor      # only meaningful in a debug build with console enabled
```

For production builds, the git commit hash is embedded at build time and
visible via `esptool.py --port /dev/cu.usbmodem* read_flash 0x10000 1024 dump.bin`
followed by `strings dump.bin | grep waypoint`. Future work may expose
this via a sentinel reserved-ID frame.
