# MRC'26 Autonomous Pathfinding Mission: survey module runbook

Operational guide for running the `survey` module (autonomous waypoint
navigation + ArUco detection) on a rover for the MRC'26 Autonomous
Pathfinding Mission. Platform pieces involved: core Autonomous mode, the
agent's `drive-control` module capability, and the dashboard's Auto mode
switch.

## What the mission needs from the rover

- Mode indicator light at all times: green = manual, red = autonomous
  transit, yellow = sensing. Missing light costs 10% of the mission score.
- Autonomously visit the given waypoints in sequence, within 50 cm each.
- At each waypoint: stop, sense the ArUco tag (5x5_100), display its ID
  visibly to referees.
- Return to base; following the reverse path earns extra points.
- Mission log (position, heading, speed, detected IDs) handed over on an
  SD card.

## One-time rover prep

### 1. Flash the dev image

Build artifacts land in `image/output/dev/images/` after
`make -C image image-dev`. Flash the sdcard image to the SD card as usual.
The dev image ships SSH (dropbear, key in the dev overlay), unsigned OTA,
and the unsigned local module override path; prod ships none of these, so
the dev image is the right choice for a competition day with on-site
iteration.

SPI for the LED strip is already enabled in the image (`dtparam=spi=on`).

### 2. Wire the WS2812 strip

- DIN → GPIO10 / SPI0 MOSI, physical pin 19.
- 5V → pin 2 or 4, GND → pin 6.
- Keep the data lead short (under ~15 cm). WS2812 nominally wants 5 V data;
  3.3 V from the Pi works reliably with a short lead. If the strip
  glitches, add a level shifter or power the strip from 3.7-4.3 V instead.
- Mount the strip horizontally and visibly; referees read it from meters
  away.

### 3. Install the survey module

Copy the module image and write the dev override (dev images only):

```sh
scp modules/survey/dist/survey-0.1.0.raw root@<rover>:/data/waypoint/store/
```

`/data/waypoint/modules.desired.override.toml` on the rover:

```toml
[[modules]]
id       = "survey"
version  = "0.1.0"
raw_path = "/data/waypoint/store/survey-0.1.0.raw"
config   = """
[modules_config.survey]
waypoints     = "..."        # from tools/mrc26/waypoints_to_local.py
tag_ids       = "..."
start_pose    = "0,0,90"
camera_device = "/dev/video0"
marker_size_m = 0.155        # measure the real tags on site
"""
```

Then `systemctl restart waypoint-agent`. Verify:

```sh
systemctl status waypoint-module-survey   # active (running)
journalctl -u waypoint-module-survey -f   # "survey: mission log" line, LED frames
ls /dev/spidev0.0 /dev/video*
```

### 4. Give the module its camera

The agent auto-claims every capture device unless `identity.toml` lists
cameras explicitly. To leave a device for the module, pin the agent's
cameras in `/data/waypoint/identity.toml`:

```toml
[[cameras]]
name   = "fpv"
device = "/dev/video1"   # the camera the AGENT keeps
```

Whatever is not listed stays free for the module (`camera_device` in the
module config). If the rover has a single camera, give it to the module:
list a nonexistent device for the agent or leave FPV without video during
the mission; the manual exit can be flown line-of-sight. Restart the agent
after editing.

## Bench test (night before)

1. Print test tags: `tools/mrc26/gen_test_tags.py --ids 11,40,17` (A4,
   print at 100% scale, verify with the 10 cm ruler on the sheet). The
   black marker square is exactly 14 cm: set `marker_size_m = 0.14` for
   bench runs only.
2. Small course config, e.g. `waypoints = "2,0;2,2"`, `tag_ids = "11,40"`,
   `start_pose = "0,0,0"`, wheels off the ground or a cleared room.
3. Dashboard → Control → Manual, verify LED green. Switch to Auto:
   LED red, wheels drive, and when a tag is in view the rover servos to it,
   stops, LED 0 yellow + white binary ID on LEDs 1-7 for the dwell.
4. Check the CSV under `/var/lib/waypoint-module-survey/logs` (on the
   store partition).
5. E-stop from the dashboard at least once during autonomous motion:
   wheels must freeze instantly (core-level, independent of the module).

## Field procedure

### Setup (before the run)

1. Get the waypoint list from the committee; save as `targets.txt`
   (`seq,lat,lon,alt` per line).
2. `python3 tools/mrc26/waypoints_to_local.py targets.txt --tag-ids ...`
   The local frame is x = East, y = North, origin = waypoint 1.
3. Measure the actual black-square side of a real competition tag during
   orientation; set `marker_size_m` (distance estimates scale with it).
4. Start pose: after teleoping out of the base, park the rover at the
   designated autonomy start point. Measure its position relative to
   waypoint 1 (tape measure or paced) and its heading with a phone compass.
   `theta_deg = 90 - compass_bearing` (compass 0=N clockwise → frame
   counterclockwise-from-East). Update `start_pose`, restart the agent,
   confirm the module log echoes the config.
5. The rover only obeys the module in Autonomous, so nothing moves until
   the dashboard switch flips: safe to prep with the rover on.

### The run

1. LED green: teleop out of the base to the start point (manual driving
   is allowed for this step; stay within the rules on touching the rover
   only inside the base).
2. Park at the measured start pose, aim at the surveyed heading.
3. Dashboard → Control → Auto. LED goes red, hands off. The dashboard
   stops publishing drive commands outside Manual, so it cannot fight the
   module.
4. Watch. At each waypoint the rover stops, LED 0 turns yellow and
   LEDs 1-7 show the tag ID in binary (MSB first) for the sensing dwell.
5. Emergency: dashboard E-stop halts the platform instantly regardless of
   the module. Recover re-arms via Safe. Switching back to Manual at any
   time freezes the module (it holds mission state and resumes if Auto is
   re-entered).
6. After the return leg the module stops at the start point and holds red
   until you switch to Manual.

### After the run

Copy the mission CSV to the laptop and onto the SD card for the referees:

```sh
scp -O root@<rover>:/data/waypoint/store/module-state/var/lib/waypoint-module-survey/logs/mission-*.csv .
```

(BusyBox scp needs `-O`.) Hand over the card together with the printed
LED legend card (`tools/mrc26/legend-card.html`).

## Troubleshooting

| Symptom | Check |
|---|---|
| LED stays green in Auto | `journalctl -u waypoint-module-survey`: is the mode mirror arriving? Agent running? `event.mode` shows AUTONOMOUS? |
| Mode switch bounces back to Safe | Core heartbeat: agent must be up; check `event.fault` for `heartbeat_lost`. |
| Rover ignores drive commands | Mode must be AUTONOMOUS (broker gates on it); check `waypoint-agent` journal for drive broker lines. |
| No detections | Camera device contested with the agent (see identity.toml step); `journalctl` shows vision child restarts; lens cap, exposure, tag distance over ~4 m. |
| Distance estimates off | `marker_size_m` doesn't match the real tag; re-measure. |
| Wheels twitch at standstill | Should not happen (direct GOAL_SPEED passthrough); if it does, E-stop and check for a second drive publisher. |
| Module dies mid-run | It restarts (systemd `Restart=always`) and resumes from config start; the platform freezes within 300 ms of silence (drive staleness watchdog). |
