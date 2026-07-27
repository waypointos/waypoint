# waypoint-core

## macOS

```
brew install cmake protobuf googletest
```

Build:
```
make core
```

Run all tests:
```
make core-test
```

Run against the agent's Unix relay (development):
```
./core/build/src/waypoint-core --socket /tmp/waypoint-nats.sock --servo-mock
```

## Coexistence with the simulator

The simulator (`sim/`) is unchanged. Both run against the agent's NATS,
publishing telemetry on the same subjects. Use one or the other, not both
at once; they will fight over `telemetry.drive` and `telemetry.motors`.

Convention: when developing core, run with `--servo-mock` and DON'T start
the sim. When developing the sim, leave core stopped.

## Pi 5 (Debian Bookworm)

```
sudo apt install cmake protobuf-compiler libprotobuf-dev libgtest-dev build-essential
```

Build & install paths are unchanged. The compiled-in default for
`--servo-port` is `/dev/ttyAMA0` (the Pi 5's primary UART pins); the
production image overrides it to `/dev/ttyACM0` (the HAT's USB serial
bridge). Adjust with `--servo-port`.

## Argv reference

```
waypoint-core
  --socket <path>        Path to the agent's NATS Unix socket
                         Default: /run/waypoint/nats.sock  (Linux)
                                  /tmp/waypoint-nats.sock  (macOS)
  --rover-id <id>        Rover id (e.g. "sim-01"). Matches identity.toml [rover].id
  --servo-port <path>    UART device. Default /dev/ttyAMA0
  --servo-mock           Use a mock UART (loopback echo). Implies --servo-port=mock.
  --log-level <lvl>      info | debug | warn | error  (default: info)
```

## Hardware smoke (Pi 5 only)

The shipping topology drives the HAT over USB: the HAT's serial bridge
enumerates as a CDC-ACM device (`/dev/ttyACM0`), and the production image
pins `--servo-port /dev/ttyACM0` accordingly. The servo bus is RS485 (A/B)
via the HAT's SP3485EN transceiver; `waypoint-core` owns the STS3215
protocol end-to-end.

Prerequisites:

- Waveshare Bus Servo Driver HAT connected to the Pi over USB
  (enumerates as `/dev/ttyACM0`).

Steps:

1. Connect the 4-wheel STS3215 chain to the HAT's bus terminals.
2. Power the bus from the 11.1 V battery.
3. `./build/src/waypoint-core --rover-id pi-01 --servo-port /dev/ttyACM0`
4. From a connected dashboard, drive the joystick. Watch wheels respond.
5. Cut the agent process. Within 500 ms, core logs `event.fault.heartbeat_lost`
   and all wheels coast to a stop. Restart agent → core re-enters manual mode.

Alternate topology: the HAT can instead sit on the GPIO header, with the
ESP32 servo-relay firmware forwarding bytes between `/dev/ttyAMA0` and the
bus. The shipped image does not use this path. For its bring-up sequence
(loopback, single-servo, soak) see `docs/setup/firmware.md`; that path also
needs the Pi UART enabled and the login getty disabled on `/dev/ttyAMA0`
(`sudo raspi-config` → Interface → Serial Port).
