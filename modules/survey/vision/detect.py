#!/usr/bin/env python3
"""ArUco detector child for the survey module.

Captures V4L2 frames and prints one JSON line per processed frame:
{"t": unix_s, "w": frame_width_px, "detections": [{"id": n, "corners": [[x, y] * 4]}]}
Empty detection lists are emitted too; the parent uses them as heartbeats.

Stdin line protocol: "SNAP /path/file.jpg" saves the most recent frame as a
JPEG at that path. Stdout stays a pure JSON channel; snapshot errors go to
stderr and never stop detection.
"""

import argparse
import json
import os
import sys
import threading
import time

import cv2

JPEG_QUALITY = 85


class SnapQueue:
    """Snapshot paths handed from the stdin reader thread to the main loop."""

    def __init__(self):
        self.lock = threading.Lock()
        self.paths = []

    def push(self, path):
        with self.lock:
            self.paths.append(path)

    def drain(self):
        with self.lock:
            out, self.paths = self.paths, []
        return out


def watch_stdin(snaps):
    for line in sys.stdin:
        line = line.strip()
        if line.startswith("SNAP "):
            snaps.push(line[5:].strip())


def save_snapshot(frame, path):
    try:
        parent = os.path.dirname(path)
        if parent:
            os.makedirs(parent, exist_ok=True)
        cv2.imwrite(path, frame, [int(cv2.IMWRITE_JPEG_QUALITY), JPEG_QUALITY])
    except Exception as e:  # noqa: BLE001 - a failed snapshot must not kill detection
        print("snapshot failed: %s" % e, file=sys.stderr)


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
    snaps = SnapQueue()
    threading.Thread(target=watch_stdin, args=(snaps,), daemon=True).start()
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
        for path in snaps.drain():
            save_snapshot(frame, path)
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
