# Drill bring-up

Ordered checklist for the first powered run of the drill module on a rover.
It establishes the two wiring signs, the height axis, the stall thresholds,
and finally the ratchet direction. Work through it in order: every step
assumes the ones before it passed.

Installing and enabling the module is a separate document,
[drill-module.md](drill-module.md).

## 1. Prerequisites

- The module is installed and reported healthy, and the dashboard shows the
  drill tab (`m-drill`) for this rover.
- A shell on the rover with the unit log following:

      journalctl -u waypoint-module-drill -f

- Both sample containers are empty and the carousel is free to turn.
- The mechanism is clear: nothing in the lift path, no tooling on the auger,
  full travel unobstructed.
- The estop is within reach of a second person, and the rover can be cut at
  the pack if it is not.

While the drill servos are not yet answering on the bus, core's estop sweep
blocks for up to about 200 ms waiting on their read timeouts (two servos, two
operations each, 50 ms per operation). That delay is expected before the
servos are fitted and disappears once they respond.

## 2. Configuration on the rover

The agent writes the module's configuration to
`/run/waypoint/modules/drill/config.toml` and its NATS credentials to
`/run/waypoint/modules/drill/creds.env`. Both live on tmpfs and are
re-materialized from the agent's cached desired state at every boot, so
editing them in place does not survive a reboot.

To change a value for real, push new configuration through the path the
module was installed by:

- Proxy: re-enable the module with the new `config_toml`, that is
  `POST /api/admin/rovers/{roverID}/modules/drill` with
  `{"version": "...", "config_toml": "..."}`.
- Local: repeat the dashboard MODULES install and supply the new config TOML
  at the confirm step.

Then restart the unit so it re-reads the file:

    systemctl restart waypoint-module-drill

The unit passes `--config /run/waypoint/modules/drill/config.toml` and
`--creds /run/waypoint/modules/drill/creds.env`, and the module honors both.
The flag wins; `WAYPOINT_MODULE_CONFIG` is the fallback when no flag is given.

On builds that predate the flag support the module reads only the environment
variable, so the configuration file is silently ignored and every setting
below stays at its default. The fallback there is a manual drop-in on the
rover. The rover root is read-only and the agent writes its own module
drop-ins under `/run`, so this one goes there too and has to be recreated
after every boot:

    # /run/systemd/system/waypoint-module-drill.service.d/99-config.conf
    [Service]
    Environment=WAYPOINT_MODULE_CONFIG=/run/waypoint/modules/drill/config.toml

followed by `systemctl daemon-reload` and a restart. Prefer updating the
module over carrying the drop-in.

## 3. Step 0: telemetry with nothing moving

Open the drill tab and read it before touching a control.

- MOTORS shows two rows, `11 lift` and `12 auger`, both with status `ok`.
  A row reading `no rd` means the read never came back: check the servo ids,
  the bus wiring, and that core is up.
- Position, speed, load, current, voltage and temperature are plausible.
  Bus voltage should match the pack. No row shows `OC!`.
- STATUS shows `homed: no`, `calibrated: no`, `halted: no`, `phase: idle`.
- HEIGHT renders `N/A` with reason `unhomed`. That is correct at this point:
  height needs a home anchor before it means anything.

Do not jog yet.

## 4. Step 1: lift_up_sign

Which raw velocity sign raises the carriage depends on gearing and motor
orientation, so it is a per-build setting rather than something the module
can infer.

Park the carriage roughly mid-travel by hand if it is not already, then hold
`jog up` in the HEIGHT card briefly (or D-pad up on a mirrored gamepad).
While the axis is unhomed the jog runs at `slow_jog_speed`, 150 raw, so the
motion is deliberately slow. Release immediately.

- Carriage rises: `lift_up_sign = 1`, the default. Continue.
- Carriage descends: set `lift_up_sign = -1`, re-apply the configuration per
  section 2, restart, and repeat this step.

Never home before this step is settled. Homing creeps toward what the module
believes is the top stop; with the sign inverted it creeps into the bottom
stop instead.

## 5. Step 2: encoder tick sign

Run this check immediately after the first successful home in step 3. It is
called out separately because its remedy is upstream, not on the rover, and
because nothing downstream catches the fault for you.

With the axis homed, jog down slightly and read `height_ticks` on the drill
tab. Ticks are counted from the top anchor at 0 and grow positive downward,
so a short jog down should produce a small positive value. That one reading
is the whole check: `height_norm` is still `N/A` with reason `uncalibrated`
at this point and says nothing either way.

`height_ticks` going negative as the carriage moves below the anchor means
the encoder counts the wrong way. Two later behaviors actively hide it:

- Travel calibration takes the absolute span between the two stops, so it
  completes normally and STATUS reads `calibrated: yes` with a plausible
  `travel_ticks`. That is not confirmation of the tick sign.
- `height_norm` is then clamped to 0 across the entire travel, so the HEIGHT
  bar never fills and the top-band interlock reads the carriage as parked at
  the top wherever it actually is. `switch` is enabled at any height instead
  of only inside the top band.

Because of the second point, step 7 must not be attempted until a downward
jog reads positive. The interlock that step relies on fails open while the
tick sign is inverted.

There is no configuration remedy today. `lift_ticks_sign` is the named
follow-up key for it. Record the observation and stop; do not improvise a
fix on the rover by rewiring the encoder or hand-editing the persisted
calibration.

## 6. Step 3: home

Press `Home` in the HEIGHT card. The lift creeps up at `slow_jog_speed`
until the top hard stop stalls the servo. The calibration note under the
card reports `homing`, then `done`.

Verify:

- STATUS shows `homed: yes`.
- `height_ticks` reads at or near 0.
- The HEIGHT reason changes from `unhomed` to `uncalibrated`.

If the carriage creeps down instead, stop and return to step 1. If it drives
into the stop and keeps pushing, or stops well short of it, the stall
thresholds need step 5 before homing is trustworthy.

## 7. Step 4: calibrate travel

`Run calibration` becomes available once the axis is homed. The procedure
creeps down to the bottom stop (phase `run_down`), then back up to the top
stop (phase `run_up`), and reports `done` with the travel span in ticks.
`Abort calibration` stops it at any point and leaves the previous span
untouched.

On `done`:

- The span is written to
  `/var/lib/waypoint-module-drill/calibration.toml` and reloaded at every
  start, so calibration survives a restart or a module update.
- STATUS shows `calibrated: yes`.
- `height_norm` goes live (0 at the top, 1 at the bottom), the HEIGHT bar
  tracks the carriage, and the `goto` slider is enabled.

## 8. Step 5: stall tuning

Only needed when home or calibration ends early (a false stall) or fails to
stop at a hard stop (a missed stall). The detector declares a stall only when
the axis is loaded and not turning and not advancing, for a sustained run of
reads at the 20 Hz read rate, so `stall_ticks = 10` is half a second of
evidence.

| Key | Default | Meaning |
|---|---|---|
| `stall_load` | 600 | Absolute load at or above which a read counts as loaded. |
| `stall_ticks` | 10 | Consecutive qualifying reads before a stall is declared. |
| `stall_speed_eps` | 20 | Absolute reported speed at or below which the servo counts as not turning. |
| `stall_delta_eps` | 8 | Absolute tick movement per read at or below which the axis counts as not advancing. |

- False stall part way along the travel, usually friction or a tight spot:
  raise `stall_load` first, then `stall_ticks`.
- Missed stall at a hard stop: lower `stall_load`. If the servo still reports
  residual speed or tick movement while pressed against the stop, raise
  `stall_speed_eps` and `stall_delta_eps` to match what it actually reports.

Change one key at a time and re-run home before re-running calibration.

`lift_overcurrent_raw` and `auger_overcurrent_raw`, both 500 raw by default,
are written to the servos at startup and are the hard backstop underneath all
of this: a trip latches a halt with reason `overcurrent`. Tune the stall keys,
not the ceilings.

## 9. Step 6: auger_drill_sign

There is no throttle control anywhere: the tab, the teleop window and the
gamepad all command the auger at full scale. The only way to run an attempt
gently is to lower the speed in the configuration, so set `drill_speed` to
roughly a third of its 800 raw default before this step, re-apply per
section 2, and restore it once the sign is settled.

Hold `drill` in the AUGER card briefly, with the auger clear of material or
barely engaged.

The screw must convey material upward, out of the hole. If it drives material
down and packs the hole, set `auger_drill_sign = -1`, re-apply, and repeat.

Settle this before any switch test: the switch rotation sign is derived from
the drill sign, where `switch_direction = "ccw"` means the opposite sign and
`"cw"` means the same sign.

## 10. Step 7: first powered ratchet test

This is the test that decides `switch_direction` and whether container
switching is viable at all.

Preconditions, all required:

- Step 2 passed: a downward jog reads positive `height_ticks`. With the tick
  sign inverted the interlock below permits the switch at any height.
- The axis is calibrated and the carriage is parked inside the top band,
  `height_norm` at or below `top_band_fraction` (0.03 by default).
- Both containers are empty.
- The auger is stopped and nothing is in the carousel path.

As in step 6 there is no throttle: `switch` runs at the full `switch_speed`,
300 raw by default. This is the first powered engagement of an unknown
ratchet, so halve `switch_speed` in the configuration first, re-apply per
section 2, and raise it again only once the direction is known good.

The `switch` button stays disabled until the interlock allows it and shows
its refusal reason when it does not. Hold it briefly and watch the
mechanism, not the screen.

- The ratchet engages and advances the carousel by one position: the default
  `switch_direction = "ccw"` is correct. Done.
- The ratchet is driven against its engagement direction, or the carousel
  does not advance: set `switch_direction = "cw"`, re-apply, and retest.
- The ratchet slips, or engages only partially, in both directions: record
  exactly what was observed and stop. The container-switch work pauses there
  for a mechanical redesign. The drill and lift functionality stands on its
  own and stays in service.

## 11. Troubleshooting

**Halt latch.** A halt zeroes both servos and cuts torque, and it latches.
Motion resumes only on a fresh input: release every control, then press
again. A button held through the fault does not clear the latch, by design,
because the gamepad mirror repeats a held button continuously.

Halt reasons, shown under `halted` in the STATUS rail:

| `halt_reason` | Means |
|---|---|
| `stop command` | The operator pressed STOP (teleop window) or the tab sent a stop. |
| `input stale` | A hold-to-move input aged past `stale_input_ms` (150 ms). The operator link dropped. |
| `read gap` | A servo stopped answering for longer than `read_gap_halt_ms` (250 ms) while it was being driven. |
| `overcurrent` | A servo reported its overcurrent flag. Clear the mechanical cause before retrying. |

Height `N/A` reasons:

| Reason | Means |
|---|---|
| `unhomed` | No top anchor yet. Run `Home`. |
| `uncalibrated` | Homed, but the travel span is unknown. Run the travel calibration. |

`height_mm` additionally needs `mm_per_tick` in the configuration; without it
the field stays `N/A` while ticks and norm are live.

Switch refusal reasons, shown under `switch` in the STATUS rail:

| `switch_refused_reason` | Means |
|---|---|
| `uncalibrated` | No travel span, so the top band cannot be judged. |
| `below top band` | The carriage is lower than `top_band_fraction`. Raise it. |
| `halted` | The halt latch is set. Clear it with a fresh input. |

Refused commands appear once under `refused` in the STATUS rail, for example
`goto_height: uncalibrated` or `calibrate: unhomed`. They refuse the command
only; they are not halts.
