# Episodes

The agent records episodes: short, labelled captures of everything the platform
observes and commands while it runs a task. It taps the same telemetry, command, component, and camera streams that drive the
dashboard and writes them to disk in a portable format so a run can be replayed,
inspected, or exported later.

An episode is two files under `/data/waypoint/episodes/`:

- `<episode_id>.mcap`: the recorded streams in an MCAP container.
- `<episode_id>.json`: a sidecar with the episode metadata.

The episode id is derived from the start time, for example
`ep-20260612T143015Z-9f3a`. The two files share that stem.

## Recording from the teleop console

Recording is driven from the teleop console.

1. Type a short task label (for example `push the block`). The label is stored on
   the episode and is how you tell runs apart later.
2. Press record to start. The recorder resolves the recording set, opens the MCAP
   file, and subscribes to the streams. The console shows elapsed time and the
   bytes written so far.
3. Press stop to finish. You mark the run as a success or a failure and may add a
   note. The success flag is optional: an episode can end without one, and a
   crashed episode never has one (see Crash recovery).

Starting a recording can be refused. The two reasons are an episode already in
progress (the recorder records one episode at a time) and low disk (see Disk
guard). The console reports the reason rather than starting.

## What gets recorded

The recording set is resolved at the moment you press record, never hardcoded. It
comes from three sources:

- **Descriptor observation streams.** Every observation stream the platform
  descriptor declares (for example `telemetry.drive`, `telemetry.motors`,
  `telemetry.power`). A platform records exactly the telemetry it actually
  produces.
- **Command streams implied by action altitudes.** Each action altitude in the
  descriptor implies the command stream that drives it: `body_twist` implies
  `cmd.drive`, `joint_position` implies `cmd.servo`. A bench with no drive train
  records `cmd.servo` but not `cmd.drive`.
- **Active component leaves.** Every installed component contributes the state and
  command leaves of its class. An `arm` component with module id `so100` adds
  `module.so100.arm.state` and `module.so100.arm.cmd`. A module installed
  mid-session is picked up by the next episode, because the set is resolved per
  episode start.

Cameras are tapped separately. Each running camera contributes one video stream
recorded as complete H.264 access units. Video is keyframe gated: the recorder
drops access units for a camera until its first keyframe arrives, so every
recorded video stream is decodable from its start. The camera tap runs on the
streaming path and must never block it, so a recorder that falls behind drops
frames and counts them in `video_frames_dropped` rather than stalling video.

A platform with no cameras records state only. The episode is still valid: it has
the telemetry, command, and component streams, just no video channel.

## Artifact format

The format is a stability contract. The sidecar carries `format_version`, which is
`1` today. A reader should check it before parsing and an exporter should refuse a
version it does not understand.

### Sidecar fields

| Field | Type | Meaning |
|---|---|---|
| `format_version` | int | sidecar schema version, `1` today |
| `episode_id` | string | id shared by the `.mcap` and `.json` |
| `platform_id` | string | descriptor platform id, for example `waypoint-rover` |
| `rover_id` | string | rover that produced the episode |
| `task_label` | string | the operator label |
| `start` | timestamp | episode start (UTC) |
| `end` | timestamp | episode end (UTC) |
| `duration_s` | number | recorded duration in seconds |
| `success` | bool or null | operator outcome; `null` is first-class (unset or crashed) |
| `notes` | string | operator note, plus an auto-stop note when applicable |
| `crashed` | bool | `true` for an episode recovered after an unclean shutdown |
| `bytes` | int | size of the `.mcap` file |
| `video_frames_dropped` | int | access units dropped under back pressure |
| `streams` | array | per-stream subject, message name, and recorded count |

Each entry in `streams` has `subject` (the recorded topic), `message` (the fully
qualified protobuf message name), and `count` (messages recorded on that stream).

### MCAP conventions

- **Topics are rover relative.** A channel topic is the subject without the
  `waypoint.<rover>.` prefix, for example `telemetry.drive` or `cmd.servo`. The
  rover id lives in the sidecar, not in every topic.
- **Messages are protobuf with embedded schemas.** Every channel uses the
  `protobuf` message encoding, and the container carries the serialized
  `FileDescriptorSet` for each message type, so a reader needs no external `.proto`
  files to decode the streams.
- **`logTime` is receive time.** The MCAP `logTime` on each message is when the
  agent received it. The authoritative capture time is the `Stamp` carried inside
  the message payload (the dual-stamp convention from the platform contract). Read
  the in-message stamp for sensor-accurate timing, `logTime` for ordering.
- **Video is `foxglove.CompressedVideo`.** Each camera is a channel named
  `camera.<id>/h264` whose messages are `foxglove.CompressedVideo` records carrying
  one H.264 Annex-B access unit each, with `format: "h264"`. The schema name is the
  vendored Foxglove name so Studio resolves the decoder.

The full sidecar is also embedded in the container as MCAP metadata named
`waypoint.episode`, so an episode file is self-describing even when separated from
its sidecar.

## Opening an episode in Foxglove Studio

An `.mcap` opens directly in [Foxglove Studio](https://foxglove.dev/) for QA. The
embedded schemas decode the protobuf streams without setup, and the
`camera.<id>/h264` channel renders as video because it uses the
`foxglove.CompressedVideo` schema. Drag the `.mcap` onto a Studio window, or open
it from the local file source.

## Disk guard

The recorder protects the data partition. It refuses to start an episode when free
space is below a threshold, and it auto-stops an in-progress episode if free space
falls below that threshold mid-run. An auto-stopped episode is finalized normally:
its bytes are kept, its `success` is left unset, and its `notes` records that it was
auto-stopped and why. An alert is raised so the auto-stop is visible.

Two environment variables tune the recorder:

| Variable | Default | Meaning |
|---|---|---|
| `WAYPOINT_EPISODES_DIR` | `/data/waypoint/episodes` | where episodes are written |
| `WAYPOINT_EPISODES_MIN_FREE_MB` | `500` | minimum free space, in MiB, to start or continue |

## Crash recovery

While an episode records, its container is written to `<episode_id>.mcap.partial`
and renamed to `<episode_id>.mcap` on a clean stop. A chunked MCAP container is
readable without its footer, so a crash or power cut leaves usable data. At boot
the agent salvages any orphaned `.partial` file: it renames the bytes in place and
writes a sidecar with `crashed: true` and `success: null`, noting that the
container has no summary index. The data is kept; the unknown outcome is honest.

## The module read seam

Exporting episodes (for example into a LeRobot dataset) is a job for a module, not
the agent. A future exporter module declares a read-only grant of the episodes
directory and discovers episodes over NATS via `rpc.episode_list`, which returns
the sidecar metadata for every recorded episode. The module reads the `.mcap` files
it cares about and writes its output elsewhere. The recorder owns the capture; the
module owns the export. The `format_version` field is the contract between them.
