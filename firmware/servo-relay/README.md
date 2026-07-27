# servo-relay

ESP-IDF firmware for the Waveshare Bus Servo Driver HAT's onboard ESP32.
Transparent, framing-aware byte relay between the Raspberry Pi's UART and
the STS3215 servo bus.

- **Setup, build, flash, bring-up:** `../../docs/setup/firmware.md`
- **Tests (host-runnable):** `cd test && make check`, or `make firmware-test`
  from the repo root.

This firmware is the lowest layer of the Pi-to-servo path; it owns no
STS3215 protocol semantics. All servo logic lives in `../../core/`.
