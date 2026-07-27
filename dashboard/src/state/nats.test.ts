// dashboard/src/state/nats.test.ts
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { Bus, decodeEnvelope, encodePub, encodeSub, matchesSubject, type NatsCred } from './nats';

describe('envelope codec', () => {
  it('encodes a subscribe envelope', () => {
    expect(encodeSub('waypoint.r.telemetry.drive')).toEqual({
      action: 'sub',
      subject: 'waypoint.r.telemetry.drive',
    });
  });

  it('encodes a publish envelope with base64 payload', () => {
    const out = encodePub('waypoint.r.cmd.drive', new Uint8Array([1, 2, 3]));
    expect(out.action).toBe('pub');
    expect(out.subject).toBe('waypoint.r.cmd.drive');
    // Round-trip: base64-decoded bytes match the original payload.
    const decoded = atob(out.data);
    expect(decoded.length).toBe(3);
    expect(decoded.charCodeAt(0)).toBe(1);
    expect(decoded.charCodeAt(1)).toBe(2);
    expect(decoded.charCodeAt(2)).toBe(3);
  });

  it('decodes an inbound envelope', () => {
    const msg = decodeEnvelope({
      action: 'msg',
      subject: 'waypoint.r.telemetry.drive',
      data: btoa('payload'),
    });
    expect(msg.subject).toBe('waypoint.r.telemetry.drive');
    expect(new TextDecoder().decode(msg.data)).toBe('payload');
  });
});

// A minimal WebSocket stub that records its constructor URL and lets the
// test drive open/close lifecycles.
class FakeWebSocket {
  static instances: FakeWebSocket[] = [];
  static OPEN = 1;
  static CLOSED = 3;
  readonly url: string;
  readyState = 0;
  onopen: (() => void) | null = null;
  onmessage: ((e: MessageEvent) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;
  sent: string[] = [];

  constructor(url: string) {
    this.url = url;
    FakeWebSocket.instances.push(this);
  }
  send(payload: string) {
    this.sent.push(payload);
  }
  close() {
    this.readyState = FakeWebSocket.CLOSED;
    this.onclose?.();
  }
}

describe('Bus.start', () => {
  let originalWs: typeof WebSocket;

  beforeEach(() => {
    FakeWebSocket.instances = [];
    originalWs = globalThis.WebSocket;
    // The WebSocket lib type carries static members (CONNECTING/OPEN/…) we
    // don't need for this test; cast is fine.
    (globalThis as unknown as { WebSocket: unknown }).WebSocket = FakeWebSocket;
  });

  afterEach(() => {
    (globalThis as unknown as { WebSocket: typeof WebSocket }).WebSocket = originalWs;
    vi.unstubAllGlobals();
  });

  it('opens the WS using natsUrl from /api/auth/nats-cred (proxy mode, no token query)', async () => {
    const cred: NatsCred = {
      jwt: 'jwt.value',
      seed: 'c2VlZA==',
      // Far enough in the future that the refresh timer floors to 30s
      // and never fires within the test.
      expiresAt: Math.floor(Date.now() / 1000) + 3600,
      natsUrl: '/ws/nats',
    };
    const fetchCred = vi.fn(async () => cred);

    const bus = new Bus(undefined, fetchCred);
    await bus.start();

    expect(fetchCred).toHaveBeenCalledTimes(1);
    expect(FakeWebSocket.instances).toHaveLength(1);
    const url = FakeWebSocket.instances[0].url;
    // jsdom defaults to http://localhost, so the proto branch picks ws:.
    expect(url.endsWith('/ws/nats')).toBe(true);
    expect(url).not.toContain('token=');
    expect(bus.getCred()).toEqual(cred);
  });

  it('passes cred to buildUrl unchanged for agent mode (natsUrl === "/ws")', async () => {
    // We don't exercise the default `buildWsUrl` here because it reaches into
    // session.ts → localStorage, which jsdom in this project doesn't expose.
    // The contract is: the Bus calls buildUrl with the credential it received.
    const cred: NatsCred = {
      jwt: 'jwt.value',
      seed: 'c2VlZA==',
      expiresAt: Math.floor(Date.now() / 1000) + 900,
      natsUrl: '/ws',
    };
    const fetchCred = vi.fn(async () => cred);
    const buildUrl = vi.fn((c: NatsCred) => `ws://localhost${c.natsUrl}?token=boot-tok`);

    const bus = new Bus(buildUrl, fetchCred);
    await bus.start();

    expect(buildUrl).toHaveBeenCalledWith(cred);
    expect(FakeWebSocket.instances).toHaveLength(1);
    expect(FakeWebSocket.instances[0].url).toBe('ws://localhost/ws?token=boot-tok');
  });
});

describe('Bus.subscribe dedup', () => {
  let originalWs: typeof WebSocket;

  beforeEach(() => {
    FakeWebSocket.instances = [];
    originalWs = globalThis.WebSocket;
    (globalThis as unknown as { WebSocket: unknown }).WebSocket = FakeWebSocket;
  });

  afterEach(() => {
    (globalThis as unknown as { WebSocket: typeof WebSocket }).WebSocket = originalWs;
    vi.unstubAllGlobals();
  });

  async function makeBus(): Promise<Bus> {
    const cred: NatsCred = {
      jwt: 'j', seed: 'c2VlZA==',
      expiresAt: Math.floor(Date.now() / 1000) + 3600,
      natsUrl: '/ws',
    };
    const bus = new Bus(() => 'ws://x/ws', async () => cred);
    await bus.start();
    const ws = FakeWebSocket.instances[0];
    ws.readyState = FakeWebSocket.OPEN;
    ws.onopen?.();
    ws.sent = []; // drop the open-flush; only inspect post-start traffic
    return bus;
  }
  function sentActions(): { action: string; subject: string }[] {
    const ws = FakeWebSocket.instances[0];
    return ws.sent.map((s) => JSON.parse(s));
  }

  it('skips redundant exact sub when a covering wildcard already exists', async () => {
    const bus = await makeBus();
    bus.subscribe('waypoint.r1.>', () => {});
    bus.subscribe('waypoint.r1.telemetry.drive', () => {});
    expect(sentActions()).toEqual([
      { action: 'sub', subject: 'waypoint.r1.>' },
    ]);
  });

  it('replaces an existing narrower sub with a covering wildcard', async () => {
    const bus = await makeBus();
    bus.subscribe('waypoint.r1.telemetry.drive', () => {});
    bus.subscribe('waypoint.r1.event.mode', () => {});
    bus.subscribe('waypoint.r1.>', () => {});
    expect(sentActions()).toEqual([
      { action: 'sub', subject: 'waypoint.r1.telemetry.drive' },
      { action: 'sub', subject: 'waypoint.r1.event.mode' },
      { action: 'unsub', subject: 'waypoint.r1.telemetry.drive' },
      { action: 'unsub', subject: 'waypoint.r1.event.mode' },
      { action: 'sub', subject: 'waypoint.r1.>' },
    ]);
  });

  it('dispatches one wildcard sub to both wildcard and exact handlers', async () => {
    const bus = await makeBus();
    const wild: Array<string> = [];
    const exact: Array<string> = [];
    bus.subscribe('waypoint.r1.>', (_d, s) => wild.push(s));
    bus.subscribe('waypoint.r1.telemetry.drive', (_d, s) => exact.push(s));
    FakeWebSocket.instances[0].onmessage?.({
      data: JSON.stringify({
        action: 'msg',
        subject: 'waypoint.r1.telemetry.drive',
        data: btoa('x'),
      }),
    } as MessageEvent);
    expect(wild).toEqual(['waypoint.r1.telemetry.drive']);
    expect(exact).toEqual(['waypoint.r1.telemetry.drive']);
  });

  it('re-subscribes uncovered patterns when a wildcard is removed', async () => {
    const bus = await makeBus();
    const unsubWild = bus.subscribe('waypoint.r1.>', () => {});
    bus.subscribe('waypoint.r1.telemetry.drive', () => {});
    bus.subscribe('waypoint.r1.event.mode', () => {});
    FakeWebSocket.instances[0].sent = [];
    unsubWild();
    const sent = sentActions();
    expect(sent[0]).toEqual({ action: 'unsub', subject: 'waypoint.r1.>' });
    const restSubs = new Set(sent.slice(1).map((e) => e.subject));
    expect(sent.slice(1).every((e) => e.action === 'sub')).toBe(true);
    expect(restSubs).toEqual(
      new Set(['waypoint.r1.telemetry.drive', 'waypoint.r1.event.mode']),
    );
  });

  it('only unsubs server-side when the last local handler is removed', async () => {
    const bus = await makeBus();
    const off1 = bus.subscribe('waypoint.r1.telemetry.drive', () => {});
    const off2 = bus.subscribe('waypoint.r1.telemetry.drive', () => {});
    FakeWebSocket.instances[0].sent = [];
    off1();
    expect(sentActions()).toEqual([]);
    off2();
    expect(sentActions()).toEqual([
      { action: 'unsub', subject: 'waypoint.r1.telemetry.drive' },
    ]);
  });
});

describe('matchesSubject', () => {
  it('exact match', () => {
    expect(matchesSubject('a.b.c', 'a.b.c')).toBe(true);
  });

  it('mismatch returns false', () => {
    expect(matchesSubject('a.b.c', 'a.b.d')).toBe(false);
  });

  it('* matches one token', () => {
    expect(matchesSubject('a.*.c', 'a.b.c')).toBe(true);
    expect(matchesSubject('a.*.c', 'a.b.x.c')).toBe(false);
  });

  it('> matches one-or-more trailing tokens', () => {
    expect(matchesSubject('waypoint.r1.>', 'waypoint.r1.telemetry.drive')).toBe(true);
    expect(matchesSubject('waypoint.r1.>', 'waypoint.r1')).toBe(false);
    expect(matchesSubject('waypoint.>', 'waypoint.r1.telemetry.drive')).toBe(true);
  });

  it('> only valid as final token — mid-pattern > does not match', () => {
    // > in the middle is a malformed pattern; for safety it should not match.
    expect(matchesSubject('a.>.b', 'a.x.b')).toBe(false);
  });
});
