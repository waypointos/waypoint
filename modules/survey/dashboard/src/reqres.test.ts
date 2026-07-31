import { createRequester, type SubscribeFn } from './reqres';

type Bridge = {
  publish: (subject: string, bytes: Uint8Array) => void;
  subscribe: SubscribeFn;
  emitResp: (payload: unknown) => void;
  published: { subject: string; msg: Record<string, unknown> }[];
};

function makeBridge(): Bridge {
  const handlers = new Map<string, (b: Uint8Array) => void>();
  const published: Bridge['published'] = [];
  return {
    publish: (subject, bytes) => {
      published.push({ subject, msg: JSON.parse(new TextDecoder().decode(bytes)) });
    },
    subscribe: (subject, onBytes) => {
      handlers.set(subject, onBytes);
      return () => handlers.delete(subject);
    },
    emitResp: (payload) => {
      handlers.get('waypoint.r1.module.survey.ui.resp')?.(
        new TextEncoder().encode(JSON.stringify(payload)),
      );
    },
    published,
  };
}

const PREFIX = 'waypoint.r1.module.survey.';

describe('createRequester', () => {
  it('correlates a response by request_id', async () => {
    const b = makeBridge();
    const r = createRequester(PREFIX, b.publish, b.subscribe);
    const p = r.request('logs.list');
    expect(b.published).toHaveLength(1);
    expect(b.published[0].subject).toBe(`${PREFIX}ui.req`);
    const { request_id, op } = b.published[0].msg;
    expect(op).toBe('logs.list');
    // A stray reply for someone else must not resolve this request.
    b.emitResp({ request_id: 'other', files: [] });
    b.emitResp({ request_id, files: [{ name: 'a.csv', size: 3 }] });
    await expect(p).resolves.toMatchObject({ files: [{ name: 'a.csv', size: 3 }] });
    r.dispose();
  });

  it('rejects on an error response', async () => {
    const b = makeBridge();
    const r = createRequester(PREFIX, b.publish, b.subscribe);
    const p = r.request('logs.get', { name: '../nope' });
    b.emitResp({ request_id: b.published[0].msg.request_id, error: 'invalid file name' });
    await expect(p).rejects.toThrow('invalid file name');
    r.dispose();
  });

  it('rejects on timeout', async () => {
    vi.useFakeTimers();
    try {
      const b = makeBridge();
      const r = createRequester(PREFIX, b.publish, b.subscribe, 50);
      const p = r.request('trail.get');
      const guarded = expect(p).rejects.toThrow('no response');
      vi.advanceTimersByTime(60);
      await guarded;
      r.dispose();
    } finally {
      vi.useRealTimers();
    }
  });

  it('passes params through to the request payload', () => {
    const b = makeBridge();
    const r = createRequester(PREFIX, b.publish, b.subscribe);
    r.request('waypoints.set', { waypoints: [[1, 2]], tag_ids: [7] }).catch(() => {});
    expect(b.published[0].msg).toMatchObject({
      op: 'waypoints.set',
      waypoints: [[1, 2]],
      tag_ids: [7],
    });
    r.dispose();
  });
});
