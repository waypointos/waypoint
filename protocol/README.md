# Waypoint protocol

The contract between every component on the Waypoint bus. All telemetry,
commands, and events cross protobuf message boundaries defined here.

## Layout

- `VERSION`                — protocol major (single integer)
- `subjects.toml`          — canonical NATS subject list
- `messages/*.proto`       — message types
- `gen/go/`                — generated Go bindings (committed)
- `gen/ts/`                — generated TS bindings (committed)
- `buf.yaml` / `buf.gen.yaml` — codegen config

## Generating bindings

    buf generate

Both Go and TS bindings regenerate. Commit the generated output — consumers
import from `protocol/gen/...`, they don't run codegen themselves.

## Adding a subject

1. Add the subject to `subjects.toml` under the appropriate group.
2. Add or extend the protobuf message in `messages/`.
3. `buf generate`.
4. Update consumers (agent, sim, dashboard).
5. If a previously-required field becomes optional or a type changes, bump `VERSION`.

## N/A handling

All telemetry fields that can be absent are `optional` in protobuf. There are
NO sentinel zeros. Consumers MUST check `.has_<field>()` (Go: `*ptr != nil` after
codegen with `optional`) and render N/A when missing.
