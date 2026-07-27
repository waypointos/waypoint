# Platform descriptor schema v1

The platform descriptor is a single TOML file describing one platform: the
single source of truth for a robot's shape. It is strictly validated, explicitly
versioned, and uses SI units and radians throughout. The canonical instance for
the Waypoint rover is `waypoint-rover.toml` in this directory; the Go parser and
validator live in `descriptor/`.

## General rules

- `schema` is required and must equal a supported version (v1 supports `1`).
- Unknown keys anywhere are hard errors. Extending the schema requires a version
  bump, not a stray key. This makes typos fail loudly instead of silently.
- All distances are metres, all angles and angular rates are radians and
  radians per second.
- Limits are required on platform-owned joints only. Module-owned joints may
  omit limits; the owning module calibrates its own (the so100 arm measures its
  hard stops itself).
- Error messages name the offending table, key, and value.

## Binding-milestone discipline

Sections with no consumer yet are full schema citizens from day one: parsed,
type-checked, exercised by golden fixtures, and rejected on error. Sections not
yet consumed at runtime are still validated so descriptors stay
forward-compatible. A field that exists only as prose is forbidden; if it is in
the schema, the validator enforces it.

## `[platform]`

| Field | Type | Required | Notes |
|---|---|---|---|
| `id` | string | yes | `[a-z][a-z0-9-]*`; matches the descriptor filename. Must be non-empty. |
| `name` | string | no | Human-readable display name. |
| `vehicle_class` | string | yes | Closed enum: `diff_drive_rover`, `fixed_base`. |

Validation: `platform.id` must be present; `vehicle_class` must be in the closed
enum. Drive behaviour binds to the vehicle class, not to the presence of
wheel-type joints: a `diff_drive_rover` has a drive subsystem, a `fixed_base`
does not.

## `[drivers.<name>]`

A table per driver. The `kind` field is the real/sim switch.

| Field | Type | Required | Notes |
|---|---|---|---|
| `kind` | string | yes | Closed enum: `sts3215`, `sim`. |
| `port` | string | conditional | Required when `kind = "sts3215"` (for example `/dev/ttyAMA0`). |
| `baud` | int | no | Bus baud rate (for example `1000000`). |

Validation: each driver's `kind` must be in the closed set; an `sts3215` driver
must declare `port`.

## `[[joints]]`

An array of joints. Each entry:

| Field | Type | Required | Notes |
|---|---|---|---|
| `name` | string | yes | `[a-z][a-z0-9_]*`; unique across all joints. |
| `driver` | string | yes | Must reference a defined `[drivers.*]` table. |
| `bus_id` | uint | yes | Unique per driver. |
| `type` | string | yes | Closed enum: `wheel`, `revolute`, `gripper`. |
| `ownership` | string | yes | Closed enum: `platform`, `module`. |
| `invert` | bool | no | Mirror-mounted joints set `true`. |
| `command_interfaces` | string[] | yes | Subset of: `position`, `velocity`, `effort`. |
| `state_interfaces` | string[] | yes | Subset of: `position`, `velocity`, `load`, `current`, `voltage`, `temperature`. |
| `[joints.limits]` | table | conditional | Required on platform-owned joints; see below. |

### `[joints.limits]`

| Field | Type | Notes |
|---|---|---|
| `velocity_radps` | float | Required on platform-owned `wheel` joints. |
| `position_min_rad` | float | Required (with max) on platform-owned `revolute`/`gripper` joints. |
| `position_max_rad` | float | Required (with min) on platform-owned `revolute`/`gripper` joints. |

### Joint ownership

`ownership = "platform"` is the data form of the drive-wheel guard hardcoded in
core and the agent servo broker. Platform-owned joints are never commandable
through the module-facing servo surface; module-owned joints are. Both
enforcers derive their deny-lists from this field.

Validation: joint names unique; `bus_id` unique per driver; `driver` references a
defined driver; `type`, `ownership`, and each `command_interfaces` /
`state_interfaces` value in their closed sets; platform-owned wheels require
`limits.velocity_radps`; platform-owned revolute and gripper joints require both
`limits.position_min_rad` and `limits.position_max_rad`.

## `[kinematics]`

Required for `diff_drive_rover`; forbidden for `fixed_base`. The vehicle class
decides: a `diff_drive_rover` descriptor without `[kinematics]` is an error, and
a `fixed_base` descriptor that declares `[kinematics]` is an error.

| Field | Type | Required | Notes |
|---|---|---|---|
| `model` | string | yes | Closed enum per vehicle class; `diff_drive` for `diff_drive_rover`. |
| `wheel_radius_m` | float | yes | Must be positive. |
| `track_width_m` | float | yes | Must be positive. |
| `wheels` | table | yes | Maps `front_left`, `front_right`, `back_left`, `back_right` to wheel joint names. |

Validation: `model` must match the vehicle class; `wheel_radius_m` and
`track_width_m` must be positive; every required wheel position must be present,
reference a known joint, and that joint must be of type `wheel`; unknown wheel
positions are errors. A `fixed_base` platform must omit this section entirely.

## `[allocation]` (validated, currently inert)

`diff_drive` derives its allocation from `[kinematics]`. This table is required
only for vehicle classes with an explicit effectiveness matrix (a future ROV),
so it is optional and empty for the v1 rover.

## `[[sensors]]`

An array of sensors. Optional; the v1 canonical rover declares none.

| Field | Type | Required | Notes |
|---|---|---|---|
| `name` | string | yes | Sensor identifier. |
| `kind` | string | yes | Closed enum: `imu`, `gps`, `camera`. |
| `frame` | string | no | Reference frame (for example `base_link`). |
| `rate_hz` | float | no | Publish rate. |

Validation: each sensor's `kind` must be in the closed set.

## `[modes]`

| Field | Type | Required | Notes |
|---|---|---|---|
| `available` | string[] | no | Mode names the platform supports; mirrors the mode machine. |

## `[observations]`

Holds an array of `[[observations.streams]]`, each declaring a telemetry stream
the platform publishes.

| Field | Type | Required | Notes |
|---|---|---|---|
| `subject` | string | yes | Leaf under `waypoint.<rover-id>.`; must exist in `protocol/subjects.toml`. |
| `message` | string | no | Fully qualified protobuf message name. |
| `rate_hz` | float | no | Nominal publish rate. |

Validation: every `observations.streams` subject must be present in
`subjects.toml`. This mirrors the dashboard's registry drift test and is enforced
by the Go conformance test against the repo contract.

## `[actions]`

| Field | Type | Required | Notes |
|---|---|---|---|
| `altitudes` | string[] | no | Command altitudes the platform accepts (for example `joint_position`, `body_twist`). |

## Drift enforcement

`testdata/<platform-id>.derived.golden.json` records expected derived values
computed from each descriptor: the name-to-id map, the platform-owned deny-list,
and wheel angular velocities for a fixed set of body twists (empty on a platform
without drive). The conformance test runs as a matrix over every platform
descriptor and checks the parser against the matching golden file. Every consumer
language runs the same fixtures, so a consumer that parses a descriptor
differently fails its conformance test.
