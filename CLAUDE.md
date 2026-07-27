# Waypoint: agent context

4-wheel rover platform. Pi 5 on-board, React dashboard, NATS bus
everywhere, hosted multi-rover proxy on Railway. Local-first; proxy is a
convenience layer.

## Where things live

| Subsystem | Path | Language |
|---|---|---|
| Subject + message contract | `protocol/` | protobuf + Go/TS bindings |
| On-rover orchestration daemon | `agent/` | Go |
| Safety-critical control daemon | `core/` | C++ |
| Simulator (dev) | `sim/` | Go |
| Multi-rover cloud | `proxy/` | Go on Railway |
| Single SPA, both local & proxy | `dashboard/` | React + Vite + TS |
| Buildroot OS image | `image/` | Buildroot + swupdate |
| ESP32 relay firmware (HAT) | `firmware/servo-relay/` | C (ESP-IDF) |
| Setup and operator guides | `docs/setup/` | markdown |

## Hard rules

- **Never write a UI element from scratch without checking `dashboard/src/ui/` first.** Read `dashboard/src/ui/README.md` before touching dashboard styling, components, or layout.
- **Never use hex colors inline.** Always tokens from `dashboard/src/ui/tokens.css`.
- **Never use Tailwind / MUI / Chakra / styled-components.** Hand-rolled CSS with tokens only.
- **Never add a new NATS subject without updating `protocol/subjects.toml` and bumping the protocol VERSION if it's a breaking change.**
- **N/A is a first-class telemetry state.** Optional protobuf fields, never sentinel zeros. UI renders muted "N/A" + a one-line reason hint.
- **C++ core changes require explicit user approval.** They reach the safety-critical path and ship rarely.
- **Don't run `git` commands without explicit user approval.** Stage and commit are user-triggered actions.
- **Don't scaffold `.keep` placeholder files in empty directories.** Let real files create the directory.
- **Never emit unsolicited bytes on the ESP32 UART0.** That line is a strict STS3215 byte pipe between Pi and servos; any non-frame bytes (log, banner, etc.) will corrupt core's parser. ESP-IDF console logging must stay disabled in production builds.

## Build / run commands

| Goal | Command |
|---|---|
| Generate protocol bindings | `cd protocol && buf generate` |
| Run agent | `cd agent && go run ./cmd/waypoint-agent` |
| Run sim | `cd sim && go run ./cmd/waypoint-sim` |
| Run dashboard dev server | `cd dashboard && pnpm dev` |
| Open UI gallery | `http://localhost:5173/ui-gallery` |
| Full test suite | `make test` |

## When working on…

- **UI / visual design:** review `dashboard/src/ui/README.md`, open `/ui-gallery`.
- **Protocol changes:** update `protocol/subjects.toml` and the relevant `.proto`, run `buf generate`, update consumers.
- **Agent or sim Go code:** TDD with `testify`. Run `go test ./...` after every change.
- **Cross-language work:** make sure regenerated bindings reach every consumer (Go in `agent/sim`, TS in `dashboard`).

## What this project is not

- Not multi-tenant SaaS; this is a single-operator fleet.
- Not real-time-OS critical; Pi running stock Linux is fine.
- Not microservices; two daemons on the rover (C++ core, Go agent), one Go service in cloud, one Go simulator.
