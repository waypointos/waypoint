#!/usr/bin/env python3
"""Add latitude/longitude columns to a survey mission CSV.

The module logs pose in the local frame (x = East, y = North, meters from
the origin used by waypoints_to_local.py). The mission rules ask for
latitude/longitude in the handed-over log, so this re-projects each row
with the same equirectangular model and writes <file>-gps.csv.

Usage: log_to_gps.py mission-20260801-101500.csv --origin 39.9017797,32.7704813
"""

import argparse
import csv
import math
import sys

EARTH_R = 6371008.8


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("file")
    ap.add_argument("--origin", required=True, help="lat,lon used as the local-frame origin")
    args = ap.parse_args()

    lat0, lon0 = (float(v.strip().rstrip("°")) for v in args.origin.split(","))
    out_path = args.file.rsplit(".", 1)[0] + "-gps.csv"

    with open(args.file, newline="") as fin, open(out_path, "w", newline="") as fout:
        rd = csv.DictReader(fin)
        if not rd.fieldnames or "x" not in rd.fieldnames or "y" not in rd.fieldnames:
            sys.exit("input does not look like a survey mission CSV")
        fields = ["latitude", "longitude"] + rd.fieldnames
        wr = csv.DictWriter(fout, fieldnames=fields)
        wr.writeheader()
        n = 0
        for row in rd:
            x, y = float(row["x"]), float(row["y"])
            lat = lat0 + math.degrees(y / EARTH_R)
            lon = lon0 + math.degrees(x / (EARTH_R * math.cos(math.radians(lat0))))
            row["latitude"] = f"{lat:.7f}"
            row["longitude"] = f"{lon:.7f}"
            wr.writerow(row)
            n += 1
    print(f"{out_path}: {n} rows")


if __name__ == "__main__":
    main()
