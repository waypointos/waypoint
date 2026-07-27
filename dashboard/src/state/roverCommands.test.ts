import { describe, it, expect, vi, beforeEach } from 'vitest';

const publish = vi.fn();
vi.mock('./nats', () => ({ getBus: () => ({ publish }) }));

import { setMode, estop, recover, startRecord, stopRecord } from './roverCommands';
import { ModeEvent } from '../../../protocol/gen/ts/messages/events_pb';
import { Mode } from '../../../protocol/gen/ts/messages/common_pb';
import { RecorderStartRequest, RecorderStopRequest } from '../../../protocol/gen/ts/messages/recorder_pb';
import { pbFromBinary } from './protobuf';

beforeEach(() => publish.mockClear());

describe('roverCommands', () => {
  it('setMode publishes a ModeEvent{to} to rpc.set_mode', () => {
    setMode('r1', 'manual');
    const [subject, bytes] = publish.mock.calls[0];
    expect(subject).toBe('waypoint.r1.rpc.set_mode');
    expect(pbFromBinary<ModeEvent>(ModeEvent, bytes).to).toBe(Mode.MANUAL);
  });

  it('setMode safe maps to MODE_SAFE', () => {
    setMode('r1', 'safe');
    expect(pbFromBinary<ModeEvent>(ModeEvent, publish.mock.calls[0][1]).to).toBe(Mode.SAFE);
  });

  it('estop publishes an empty payload to rpc.estop', () => {
    estop('r1');
    expect(publish.mock.calls[0][0]).toBe('waypoint.r1.rpc.estop');
    expect(publish.mock.calls[0][1].length).toBe(0);
  });

  it('recover publishes an empty payload to rpc.recover', () => {
    recover('r1');
    expect(publish.mock.calls[0][0]).toBe('waypoint.r1.rpc.recover');
    expect(publish.mock.calls[0][1].length).toBe(0);
  });

  it('startRecord publishes a RecorderStartRequest{taskLabel} to rpc.recorder_start', () => {
    startRecord('r1', 'push the block');
    const [subject, bytes] = publish.mock.calls[0];
    expect(subject).toBe('waypoint.r1.rpc.recorder_start');
    expect(pbFromBinary<RecorderStartRequest>(RecorderStartRequest, bytes).taskLabel).toBe('push the block');
  });

  it('stopRecord publishes a RecorderStopRequest{success,notes} to rpc.recorder_stop', () => {
    stopRecord('r1', true, 'all good');
    const [subject, bytes] = publish.mock.calls[0];
    expect(subject).toBe('waypoint.r1.rpc.recorder_stop');
    const req = pbFromBinary<RecorderStopRequest>(RecorderStopRequest, bytes);
    expect(req.success).toBe(true);
    expect(req.notes).toBe('all good');
  });

  it('stopRecord defaults notes to an empty string', () => {
    stopRecord('r1', false);
    expect(pbFromBinary<RecorderStopRequest>(RecorderStopRequest, publish.mock.calls[0][1]).notes).toBe('');
  });
});
