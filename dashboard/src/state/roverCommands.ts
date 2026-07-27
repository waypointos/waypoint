// dashboard/src/state/roverCommands.ts
//
// Fire-and-forget rover control commands over the NATS bus. Mode/estop/recover
// are published to the core's rpc.* subjects; confirmation comes back on the
// rover's event.mode stream (RoverView already subscribes), so we never wait on
// a reply. The core handlers ignore the request body for estop/recover and read
// `to` for set_mode (they reuse ModeEvent — no dedicated request message exists).
import { Mode } from '../../../protocol/gen/ts/messages/common_pb';
import { ModeEvent } from '../../../protocol/gen/ts/messages/events_pb';
import { RecorderStartRequest, RecorderStopRequest } from '../../../protocol/gen/ts/messages/recorder_pb';
import { getBus } from './nats';
import { pbToBinary } from './protobuf';

const EMPTY = new Uint8Array(0);

export function setMode(roverId: string, target: 'manual' | 'safe'): void {
  const ev = new ModeEvent({ to: target === 'manual' ? Mode.MANUAL : Mode.SAFE });
  getBus().publish(`waypoint.${roverId}.rpc.set_mode`, pbToBinary(ev));
}

export function estop(roverId: string): void {
  getBus().publish(`waypoint.${roverId}.rpc.estop`, EMPTY);
}

export function recover(roverId: string): void {
  getBus().publish(`waypoint.${roverId}.rpc.recover`, EMPTY);
}

// Episode recorder controls. Like setMode, these are optimistic publishes to
// the agent's rpc.* subjects; confirmation arrives on the rover's
// event.recorder stream (no request/reply over the WS bridge).
export function startRecord(roverId: string, taskLabel: string): void {
  const req = new RecorderStartRequest({ taskLabel });
  getBus().publish(`waypoint.${roverId}.rpc.recorder_start`, pbToBinary(req));
}

export function stopRecord(roverId: string, success: boolean, notes = ''): void {
  const req = new RecorderStopRequest({ success, notes });
  getBus().publish(`waypoint.${roverId}.rpc.recorder_stop`, pbToBinary(req));
}
