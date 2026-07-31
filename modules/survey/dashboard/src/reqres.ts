// Request/reply emulation over the pub/sub-only panel bridge: requests go
// to ...ui.req with a request_id, the module publishes matching replies on
// ...ui.resp, and this correlates them back into promises.
export type PublishFn = (subject: string, bytes: Uint8Array) => void;
export type SubscribeFn = (subject: string, onBytes: (b: Uint8Array) => void) => () => void;

export type Requester = {
  request: (op: string, params?: Record<string, unknown>) => Promise<Record<string, unknown>>;
  dispose: () => void;
};

type Pending = {
  resolve: (v: Record<string, unknown>) => void;
  reject: (e: Error) => void;
  timer: ReturnType<typeof setTimeout>;
};

export function createRequester(
  prefix: string,
  publish: PublishFn,
  subscribe: SubscribeFn,
  timeoutMs = 10000,
): Requester {
  const pending = new Map<string, Pending>();
  const enc = new TextEncoder();
  const dec = new TextDecoder();
  let seq = 0;

  const unsub = subscribe(`${prefix}ui.resp`, (bytes) => {
    let msg: Record<string, unknown>;
    try {
      msg = JSON.parse(dec.decode(bytes));
    } catch {
      return;
    }
    const id = typeof msg.request_id === 'string' ? msg.request_id : '';
    const p = pending.get(id);
    if (!p) return;
    pending.delete(id);
    clearTimeout(p.timer);
    if (msg.error) p.reject(new Error(String(msg.error)));
    else p.resolve(msg);
  });

  function request(op: string, params: Record<string, unknown> = {}): Promise<Record<string, unknown>> {
    const request_id = `${Date.now().toString(36)}-${++seq}`;
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => {
        pending.delete(request_id);
        reject(new Error(`${op}: no response from module`));
      }, timeoutMs);
      pending.set(request_id, { resolve, reject, timer });
      try {
        publish(`${prefix}ui.req`, enc.encode(JSON.stringify({ request_id, op, ...params })));
      } catch (e) {
        pending.delete(request_id);
        clearTimeout(timer);
        reject(e instanceof Error ? e : new Error(String(e)));
      }
    });
  }

  function dispose() {
    unsub();
    for (const p of pending.values()) {
      clearTimeout(p.timer);
      p.reject(new Error('disposed'));
    }
    pending.clear();
  }

  return { request, dispose };
}
