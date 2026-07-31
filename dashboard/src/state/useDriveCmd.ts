// dashboard/src/state/useDriveCmd.ts
//
// Publishes DriveCommand at ~50 Hz to waypoint.<rover>.cmd.drive while the
// operator is actively driving. Caller sets the target (vx, yaw) in body-frame
// normalized units; physical scaling lives in ./kinematics.ts so the dashboard
// can also display per-wheel targets using the same conversion the rover applies.
import { useEffect, useRef } from 'react';
import { Timestamp } from '@bufbuild/protobuf';
import { DriveCommand } from '../../../protocol/gen/ts/messages/drive_pb';
import type { Mode as DisplayMode } from '../ui/telemetry/ModeIndicator';
import { getBus } from './nats';
import { pbToBinary } from './protobuf';
import { stickToPhysical } from './kinematics';

type Options = {
  /**
   * When true, no DriveCommand frames are published. Use from views mounted
   * under monitor-role accounts so they never claim write authority on
   * cmd.drive, even with a zero stick.
   */
  paused?: boolean;
  /**
   * Confirmed mode from the rover's event.mode stream, not the optimistic
   * pending intent. Frames are only published in Manual; in Autonomous the
   * autonomy module owns cmd.drive and the dashboard must not compete.
   */
  mode: DisplayMode;
};

// Keep streaming briefly after the stick centres so the rover sees a clean
// trailing zero, then go silent and let core's drive-staleness failsafe hold
// the stop. Idle dashboards stay quiet so multiple open tabs don't interleave
// zeros with another tab's real commands.
const IDLE_GRACE_MS = 200;

export function useDriveCmd(
  roverId: string,
  target: { vx: number; yaw: number },
  options: Options,
) {
  const subject = `waypoint.${roverId}.cmd.drive`;
  const ref = useRef(target);
  ref.current = target;
  const lastActiveMs = useRef(0);

  const { paused = false, mode } = options;

  useEffect(() => {
    if (paused || mode !== 'manual') return;
    const id = setInterval(() => {
      const t = ref.current;
      const now = Date.now();
      if (t.vx !== 0 || t.yaw !== 0) lastActiveMs.current = now;
      if (now - lastActiveMs.current > IDLE_GRACE_MS) return;
      const { vxMps, yawRadps } = stickToPhysical(t);
      const cmd = new DriveCommand({
        t: Timestamp.fromDate(new Date()),
        bodyVxMps: vxMps,
        yawRateRadps: yawRadps,
      });
      getBus().publish(subject, pbToBinary(cmd));
    }, 20); // 50 Hz
    return () => clearInterval(id);
  }, [subject, paused, mode]);
}
