# Changelog

Generated from conventional commits by the release targets in the root
Makefile. Do not edit by hand.

## 0.1.0 (2026-07-27)

First versioned release of the rover OS image. Development before this
point predates conventional commits, so this entry summarizes the
platform instead of listing commits. Later entries are derived by
`make release-image`.

- Buildroot A/B image for the Pi 5 with signed swupdate OTA (cosign
  keyless, built and signed by CI on `image-v*` tags) and automatic
  boot fallback.
- Go agent: embedded NATS bus and HTTP gateway, proxy uplink, camera
  auto-discovery, episodic MCAP recording, and the module system
  (portable services, offline cosign verification, component
  auto-grant, sandboxed subjects).
- C++ core: descriptor-driven safety-critical control, STS3215 servo
  bus, drive staleness failsafe, estop handling with module wheel
  sweep, and the servo broker for modules.
- React dashboard embedded in the agent: fleet and rover views, teleop
  console, module tabs, releases and marketplace views.
- Shared platform descriptor and protobuf protocol across agent, core,
  sim, proxy, and dashboard.
- Platform openings for external module components: open component
  class registry, signed servo goal velocity, generic state and cmd
  leaves.
