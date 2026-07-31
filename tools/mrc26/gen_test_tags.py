#!/usr/bin/env python3
"""Generate printable ArUco test tags matching the MRC target style.

Produces A4-proportioned 300 DPI PNGs, one tag per page: a 5x5_100 marker
with a colored border frame and a 10 cm ruler bar to verify print scale.
Print at 100% / actual size. The black marker square is exactly
MARKER_CM wide; use that value for the module's marker_size_m when bench
testing (measure the real competition tags on site and update the config).

Requires opencv-contrib-python(-headless) and numpy.

Usage: gen_test_tags.py --ids 11,40,17 --out /tmp/tags
"""

import argparse
import os

import cv2
import numpy as np

DPI = 300
A4_W, A4_H = 2480, 3508
MARKER_CM = 14.0
BORDER_CM = 1.5
COLORS_BGR = {"red": (40, 40, 220), "green": (70, 160, 40), "blue": (200, 90, 30)}
SEQ = ["red", "green", "blue"]


def cm_px(cm):
    return int(round(cm / 2.54 * DPI))


def make_page(tag_id, color_name):
    dic = cv2.aruco.getPredefinedDictionary(cv2.aruco.DICT_5X5_100)
    side = cm_px(MARKER_CM)
    marker = cv2.aruco.generateImageMarker(dic, tag_id, side)

    page = np.full((A4_H, A4_W, 3), 255, np.uint8)
    quiet = cm_px(1.0)
    border = cm_px(BORDER_CM)
    frame_side = side + 2 * (quiet + border)
    x0 = (A4_W - frame_side) // 2
    y0 = cm_px(3.0)

    bgr = COLORS_BGR[color_name]
    cv2.rectangle(page, (x0, y0), (x0 + frame_side, y0 + frame_side), bgr, -1)
    cv2.rectangle(page, (x0 + border, y0 + border),
                  (x0 + frame_side - border, y0 + frame_side - border),
                  (255, 255, 255), -1)
    mx, my = x0 + border + quiet, y0 + border + quiet
    page[my:my + side, mx:mx + side] = cv2.cvtColor(marker, cv2.COLOR_GRAY2BGR)

    label = f"ID {tag_id}  ({color_name})  marker {MARKER_CM:.0f} cm"
    cv2.putText(page, label, (x0, y0 + frame_side + cm_px(1.2)),
                cv2.FONT_HERSHEY_SIMPLEX, 2.0, (0, 0, 0), 4, cv2.LINE_AA)

    rx, ry = x0, y0 + frame_side + cm_px(2.0)
    cv2.rectangle(page, (rx, ry), (rx + cm_px(10.0), ry + cm_px(0.5)), (0, 0, 0), -1)
    cv2.putText(page, "10 cm scale check", (rx, ry + cm_px(1.5)),
                cv2.FONT_HERSHEY_SIMPLEX, 1.5, (0, 0, 0), 3, cv2.LINE_AA)
    return page


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--ids", default="11,40,17")
    ap.add_argument("--out", default="test-tags")
    args = ap.parse_args()
    os.makedirs(args.out, exist_ok=True)
    ids = [int(v) for v in args.ids.split(",")]
    for i, tag_id in enumerate(ids):
        color = SEQ[i % len(SEQ)]
        path = os.path.join(args.out, f"tag-{i + 1:02d}-id{tag_id}-{color}.png")
        cv2.imwrite(path, make_page(tag_id, color))
        print(path)


if __name__ == "__main__":
    main()
