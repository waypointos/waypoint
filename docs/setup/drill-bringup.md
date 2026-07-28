# Drill bring-up

Ordered checklist for the first powered run of the drill module on a rover.
It confirms the two wiring signs, establishes the height axis, and finally
settles the ratchet direction. Work through it in order: every step assumes
the ones before it passed.

The lift has no hard stop at either end. Nothing in the module seeks a limit
under power; both ends of travel are marked by the operator, by jogging to the
end and pressing a button that records the position where the carriage already
stands.

Installing and enabling the module is a separate document,
[drill-module.md](drill-module.md).

## 1. Prerequisites

- The module is installed and reported healthy, and the dashboard shows the
  drill tab (`m-drill`) for this rover.
- A shell on the rover with the unit log following:

      journalctl -u waypoint-module-drill -f

- All three sample containers are empty and the carousel is free to turn.
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
  height needs a top anchor before it means anything.

Do not jog yet.

## 4. Step 1: lift_up_sign

`lift_up_sign` defaults to `-1`, which is correct for the reference assembly.
This step confirms it on the machine in front of you rather than discovering
it, and the key stays configurable so a one-off rebuild can correct itself.

Park the carriage roughly mid-travel by hand if it is not already, then hold
`jog up` in the HEIGHT card briefly (or D-pad up on a mirrored gamepad).
While the axis is unhomed the jog runs at `slow_jog_speed`, 150 raw, so the
motion is deliberately slow. Release immediately.

- Carriage rises: `lift_up_sign = -1`, the default. Continue.
- Carriage descends: set `lift_up_sign = 1`, re-apply the configuration per
  section 2, restart, and repeat this step.

Jog is hold-to-move and ages out on its own deadman, so it stops when you
release it. Confirm the sign here, where a wrong answer costs a fraction of a
second of travel, rather than later.

## 5. Step 2: encoder tick sign

Run this check immediately after the top is marked in step 3. It is called
out separately because its remedy is upstream, not on the rover.

With the top marked, jog down slightly and read `height_ticks` on the drill
tab. Ticks are counted from the top anchor at 0 and grow positive downward,
so a short jog down should produce a small positive value. That one reading
is the whole check: `height_norm` is still `N/A` with reason `uncalibrated`
at this point and says nothing either way.

`height_ticks` going negative as the carriage moves below the anchor means
the encoder counts the wrong way. Marking the bottom in step 4 then refuses
with `bottom is not below the top anchor`, so the fault surfaces there rather
than being stored as a plausible-looking span.

If it does, `height_norm` never goes live, so the top-band interlock stays
closed and step 6 cannot be attempted. That is the intended outcome: the
interlock refuses rather than failing open.

There is no configuration remedy today. `lift_ticks_sign` is the named
follow-up key for it. Record the observation and stop; do not improvise a
fix on the rover by rewiring the encoder or hand-editing the persisted
calibration.

## 6. Step 3: mark the top

Jog the carriage to the highest position you want it to reach, then release
and press `Set top here` in the HEIGHT card. The mark commands no servo: it
records the position the carriage is already standing at as height 0. The
calibration note under the card reports `top_set`.

Choose the position deliberately. It is the zero every later height reading
and the top-band interlock are measured from, and you will re-mark it at
roughly the same place after every restart.

Verify:

- STATUS shows `homed: yes`.
- `height_ticks` reads at or near 0.
- The HEIGHT reason changes from `unhomed` to `uncalibrated`.

The mark is refused, with the reason under `refused` in the STATUS rail, if
the lift is still moving, if the halt latch is set, or before the first servo
read has landed. Release the jog, clear any halt, and press again.

## 7. Step 4: mark the bottom

`Set bottom here` becomes available once the top is marked. Jog the carriage
down to the lowest position you want it to reach, release, and press it. The
span between the two marks is the travel, reported as `bottom_set` with the
tick count.

On `bottom_set`:

- The span is written to
  `/var/lib/waypoint-module-drill/calibration.toml` and reloaded at every
  start, so it survives a restart or a module update.
- STATUS shows `calibrated: yes`.
- `height_norm` goes live (0 at the top, 1 at the bottom), the HEIGHT bar
  tracks the carriage, and the `goto` slider is enabled.

A bottom marked at or above the top anchor is refused with `bottom is not
below the top anchor` and nothing is stored. That means either the two marks
were made in the wrong order or the encoder counts the wrong way; see step 2.

Re-marking the top keeps a stored span, because the span is a property of the
machine rather than of the anchor. The encoder loses its reference at every
restart, so marking the top is the routine per-session step and re-measuring
the span is not. Mark the bottom again whenever the top is set somewhere
other than its usual place.

## 8. Step 5: auger_drill_sign

`auger_drill_sign` defaults to `-1`, correct for the reference assembly. As
with the lift sign, this step confirms it rather than discovering it.

Use the auger speed slider to run this attempt gently. It sits in the AUGER
card and in the teleop window, and it scales `drill_speed` from 5 to 100 per
cent, so drop it to around 25 before this step and raise it once the sign is
settled. A hold re-publishes every 50 ms, so the slider also takes effect
mid-run. The gamepad D-pad ignores it and always commands full scale, so keep
the pad out of this step.

Hold `drill` in the AUGER card briefly, with the auger clear of material or
barely engaged.

The screw must convey material upward, out of the hole. If it drives material
down and packs the hole, set `auger_drill_sign = 1`, re-apply, and repeat.

Settle this before any switch test: the switch rotation sign is derived from
the drill sign, where `switch_direction = "ccw"` means the opposite sign and
`"cw"` means the same sign.

## 9. Step 6: first powered ratchet test

This is the test that decides `switch_direction` and whether container
switching is viable at all.

Preconditions, all required:

- Step 2 passed: a downward jog reads positive `height_ticks`.
- The axis is calibrated and the carriage is parked inside the top band,
  `height_norm` at or below `top_band_fraction` (0.03 by default).
- All three containers are empty.
- The auger is stopped and nothing is in the carousel path.

The speed slider scales `switch` too, against `switch_speed` rather than
`drill_speed`, so even 100 per cent is only 300 raw by default. This is the
first powered engagement of an unknown ratchet: set the slider to its 5 per
cent minimum, and raise it only once the direction is known good.

The `switch` button stays disabled until the interlock allows it and shows
its refusal reason when it does not. Hold it briefly and watch the
mechanism, not the screen.

The mechanical interlock that is meant to hold the drill core against rotation
while it is lowered is not confirmed by the CAD solids: the barrel bore is a
constant radius at every angle and height, and the core's flats clear it by
1.00 mm, so nothing keys the two together. Watch the core itself during this
test and stop if it turns with the carousel. The software top-band interlock
is a separate mechanism and is unaffected.

- The ratchet engages and advances the carousel by one position: the default
  `switch_direction = "ccw"` is correct. Done.
- The ratchet is driven against its engagement direction, or the carousel
  does not advance: set `switch_direction = "cw"`, re-apply, and retest.
- The ratchet slips, or engages only partially, in both directions: record
  exactly what was observed and stop. The container-switch work pauses there
  for a mechanical redesign. The drill and lift functionality stands on its
  own and stays in service.

## 10. Troubleshooting

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
| `unhomed` | No top anchor yet. Press `Set top here`. |
| `uncalibrated` | Top marked, but the travel span is unknown. Press `Set bottom here`. |

`height_mm` additionally needs `mm_per_tick` in the configuration; without it
the field stays `N/A` while ticks and norm are live.

Switch refusal reasons, shown under `switch` in the STATUS rail:

| `switch_refused_reason` | Means |
|---|---|
| `uncalibrated` | No travel span, so the top band cannot be judged. |
| `below top band` | The carriage is lower than `top_band_fraction`. Raise it. |
| `halted` | The halt latch is set. Clear it with a fresh input. |

Refused commands appear once under `refused` in the STATUS rail. They refuse
the command only; they are not halts.

| `refused` | Means |
|---|---|
| `goto_height: uncalibrated` | No travel span, so a normalized target has no meaning. |
| `set_bottom: unhomed, mark the top first` | The span is measured from the top anchor, which does not exist yet. |
| `set_bottom: bottom is not below the top anchor` | The marks are out of order, or the encoder counts the wrong way. See step 2. |
| `set_top: lift is moving` / `set_bottom: lift is moving` | A mark records where the carriage stands, so it will not run against a moving one. Release the jog. |
| `set_top: halted` / `set_bottom: halted` | The halt latch is set. Clear it with a fresh input. |
| `set_top: no servo read yet` | No position has arrived from the bus. Check MOTORS in step 0. |
