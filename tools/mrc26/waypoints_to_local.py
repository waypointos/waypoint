#!/usr/bin/env python3
"""Convert MRC waypoint GPS coordinates to the survey module's local frame.

The module navigates in a flat local frame: x = East, y = North, in meters,
with the origin at a reference point (default: waypoint 1). Relative geometry
from lat/lon is centimeter-accurate at field scale; the absolute anchor comes
from the operator's start-pose measurement (see the mission runbook).

Input file: one waypoint per line, "seq,lat,lon[,alt]" (alt ignored).
Lines starting with # are skipped. Degrees may carry a trailing degree sign.

Usage:
  waypoints_to_local.py targets.txt
  waypoints_to_local.py targets.txt --origin 39.9017797,32.7704813
  waypoints_to_local.py targets.txt --tag-ids 11,40,17,65,72,77,93,93,1,60
"""

import argparse
import math
import sys

EARTH_R = 6371008.8


def parse_line(line):
    parts = [p.strip().rstrip("°") for p in line.split(",")]
    if len(parts) < 3:
        raise ValueError(f"expected 'seq,lat,lon[,alt]', got: {line!r}")
    return int(parts[0]), float(parts[1]), float(parts[2])


def to_enu(lat, lon, lat0, lon0):
    x = math.radians(lon - lon0) * EARTH_R * math.cos(math.radians(lat0))
    y = math.radians(lat - lat0) * EARTH_R
    return x, y


def main():
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("file", help="waypoint list: seq,lat,lon[,alt] per line")
    ap.add_argument("--origin", help="lat,lon of the local-frame origin "
                                     "(default: waypoint with lowest seq)")
    ap.add_argument("--tag-ids", default="",
                    help="comma list of expected ArUco IDs in waypoint order")
    args = ap.parse_args()

    wps = []
    with open(args.file) as f:
        for line in f:
            line = line.strip().strip("()")
            if not line or line.startswith("#"):
                continue
            wps.append(parse_line(line))
    if not wps:
        sys.exit("no waypoints parsed")
    wps.sort(key=lambda w: w[0])

    if args.origin:
        lat0, lon0 = (float(v.strip().rstrip("°")) for v in args.origin.split(","))
    else:
        lat0, lon0 = wps[0][1], wps[0][2]

    pts = [(seq, *to_enu(lat, lon, lat0, lon0)) for seq, lat, lon in wps]

    print(f"# origin lat,lon = {lat0}, {lon0}   frame: x=East y=North (m)")
    print(f"{'seq':>4} {'x_east':>9} {'y_north':>9} {'leg_m':>7} {'bearing_deg':>11}")
    prev = (0.0, 0.0)
    total = 0.0
    for seq, x, y in pts:
        dx, dy = x - prev[0], y - prev[1]
        leg = math.hypot(dx, dy)
        total += leg
        brg = (math.degrees(math.atan2(dx, dy)) + 360) % 360
        print(f"{seq:>4} {x:>9.2f} {y:>9.2f} {leg:>7.2f} {brg:>11.1f}")
        prev = (x, y)
    print(f"# total outbound path: {total:.1f} m")

    wp_str = ";".join(f"{x:.2f},{y:.2f}" for _, x, y in pts)
    print("\n# paste into the survey module config:")
    print(f'waypoints = "{wp_str}"')
    if args.tag_ids:
        print(f'tag_ids = "{args.tag_ids}"')
    print('# start_pose = "x,y,theta_deg", theta measured counterclockwise'
          ' from East (compass bearing B -> theta = 90 - B)')


if __name__ == "__main__":
    main()
