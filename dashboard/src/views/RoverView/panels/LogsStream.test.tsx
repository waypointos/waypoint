import { vi } from 'vitest';

const { busHandlers } = vi.hoisted(() => ({
  busHandlers: new Map<string, Set<(d: Uint8Array, s: string) => void>>(),
}));

vi.mock('@/state/nats', () => ({
  getBus: () => ({
    subscribe: (subject: string, h: (d: Uint8Array, s: string) => void) => {
      let set = busHandlers.get(subject);
      if (!set) busHandlers.set(subject, (set = new Set()));
      set.add(h);
      return () => set!.delete(h);
    },
    publish: () => {},
  }),
}));

import { describe, it, expect, beforeEach } from 'vitest';
import { act, render, screen, fireEvent } from '@testing-library/react';
import { LogsStream } from './LogsStream';
import { RoverContextProvider, type RoverContextValue } from '../RoverContext';
import { LogRecord } from '../../../../../protocol/gen/ts/messages/diag_pb';
import { pbToBinary } from '@/state/protobuf';

function ctx(): RoverContextValue {
  return {
    id: 'demo', role: 'monitor', canControl: false,
    drive: null, power: null, sys: null, uplink: null, modules: [], image: null, motors: {},
    cameraNames: [], whepBase: '', roverAlerts: [],
    alertCounts: { info: 0, warning: 0, critical: 0 } as any,
    connection: { kind: 'offline' }, mode: 'unknown',
    recorder: null,
  };
}

function emitLog(rec: Partial<{ level: string; msg: string; source: string }>) {
  const bytes = pbToBinary(new LogRecord({ tsNs: BigInt(Date.now()) * 1_000_000n, ...rec }));
  const subject = 'waypoint.demo.diag.log';
  act(() => {
    for (const h of busHandlers.get(subject) ?? []) h(bytes, subject);
  });
}

function renderStream() {
  return render(
    <RoverContextProvider value={ctx()}><LogsStream /></RoverContextProvider>,
  );
}

describe('LogsStream', () => {
  beforeEach(() => busHandlers.clear());

  it('renders incoming log messages', () => {
    renderStream();
    emitLog({ level: 'info', msg: 'arm request accepted', source: 'agent.drive' });
    expect(screen.getByText('arm request accepted')).toBeInTheDocument();
  });

  it('trims trailing newlines from messages', () => {
    renderStream();
    emitLog({ level: 'info', msg: 'noisy line\n\n' });
    expect(screen.getByText('noisy line')).toBeInTheDocument();
  });

  it('hides entries below the selected minimum level', () => {
    renderStream();
    emitLog({ level: 'debug', msg: 'verbose detail' });
    emitLog({ level: 'error', msg: 'boom' });
    expect(screen.queryByText('verbose detail')).not.toBeInTheDocument();
    expect(screen.getByText('boom')).toBeInTheDocument();
  });

  it('grep filters by message substring', () => {
    renderStream();
    emitLog({ level: 'info', msg: 'voltage sag' });
    emitLog({ level: 'info', msg: 'camera started' });
    fireEvent.change(screen.getByPlaceholderText('grep…'), { target: { value: 'voltage' } });
    expect(screen.getByText('voltage sag')).toBeInTheDocument();
    expect(screen.queryByText('camera started')).not.toBeInTheDocument();
  });

  it('pauses ingestion', () => {
    renderStream();
    fireEvent.click(screen.getByText('PAUSE'));
    emitLog({ level: 'error', msg: 'dropped while paused' });
    expect(screen.queryByText('dropped while paused')).not.toBeInTheDocument();
  });

  it('filters by selected source', () => {
    renderStream();
    emitLog({ level: 'info', msg: 'from drive', source: 'agent.drive' });
    emitLog({ level: 'info', msg: 'from power', source: 'agent.power' });
    fireEvent.change(screen.getByRole('combobox'), { target: { value: 'agent.drive' } });
    expect(screen.getByText('from drive')).toBeInTheDocument();
    expect(screen.queryByText('from power')).not.toBeInTheDocument();
  });

  it('resets the source filter on clear so new logs are not silently hidden', () => {
    renderStream();
    emitLog({ level: 'info', msg: 'from drive', source: 'agent.drive' });
    fireEvent.change(screen.getByRole('combobox'), { target: { value: 'agent.drive' } });
    fireEvent.click(screen.getByText('clear'));
    emitLog({ level: 'info', msg: 'after clear', source: 'agent.power' });
    expect(screen.getByText('after clear')).toBeInTheDocument();
  });
});
