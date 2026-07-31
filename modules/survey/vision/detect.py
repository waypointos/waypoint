#!/usr/bin/env python3
"""ArUco detector child for the survey module.

Captures V4L2 frames and prints one JSON line per processed frame:
{"t": unix_s, "w": frame_width_px, "detections": [{"id": n, "corners": [[x, y] * 4]}]}
Empty detection lists are emitted too; the parent uses them as heartbeats.
"""

import argparse
import json
import sys
import time

import cv2


def make_detector():
    dic = cv2.aruco.getPredefinedDictionary(cv2.aruco.DICT_5X5_100)
    # OpenCV >= 4.7 has the ArucoDetector class; older contrib builds only
    # have the module-level function.
    if hasattr(cv2.aruco, "ArucoDetector"):
        det = cv2.aruco.ArucoDetector(dic, cv2.aruco.DetectorParameters())
        return det.detectMarkers
    params = cv2.aruco.DetectorParameters_create()
    return lambda img: cv2.aruco.detectMarkers(img, dic, parameters=params)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--device", default="/dev/video0")
    ap.add_argument("--width", type=int, default=1280)
    ap.add_argument("--height", type=int, default=720)
    ap.add_argument("--fps", type=float, default=10.0)
    args = ap.parse_args()

    cap = cv2.VideoCapture(args.device, cv2.CAP_V4L2)
    cap.set(cv2.CAP_PROP_FOURCC, cv2.VideoWriter_fourcc(*"MJPG"))
    cap.set(cv2.CAP_PROP_FRAME_WIDTH, args.width)
    cap.set(cv2.CAP_PROP_FRAME_HEIGHT, args.height)
    if not cap.isOpened():
        print("cannot open %s" % args.device, file=sys.stderr)
        return 1

    detect = make_detector()
    period = 1.0 / max(args.fps, 1.0)
    fails = 0
    while True:
        t0 = time.time()
        ok, frame = cap.read()
        if not ok:
            fails += 1
            print("frame read failed (%d)" % fails, file=sys.stderr)
            if fails >= 20:
                # Exit so the parent restarts the whole capture pipeline.
                return 1
            time.sleep(0.5)
            continue
        fails = 0
        gray = cv2.cvtColor(frame, cv2.COLOR_BGR2GRAY)
        corners, ids, _ = detect(gray)
        dets = []
        if ids is not None:
            for mid, quad in zip(ids.flatten(), corners):
                pts = [[float(x), float(y)] for x, y in quad.reshape(4, 2)]
                dets.append({"id": int(mid), "corners": pts})
        print(json.dumps({"t": t0, "w": int(gray.shape[1]), "detections": dets}), flush=True)
        dt = time.time() - t0
        if dt < period:
            time.sleep(period - dt)


if __name__ == "__main__":
    sys.exit(main() or 0)
