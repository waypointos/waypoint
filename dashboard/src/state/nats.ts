// dashboard/src/state/nats.ts
//
// Browser-side NATS subscription transport. We don't use nats.ws directly here
// because the agent's WebSocket gateway already brokers per-rover subjects and
// enforces auth; we just speak its JSON envelope.
//
// Auth model (Phase 6):
//   1. On mount we POST /api/auth/nats-cred (cookie auth in proxy mode,
//      bootstrap-token + cookie in agent mode) and receive
//      { jwt, seed, expiresAt, natsUrl }.
//   2. natsUrl tells us where the WS bridge lives:
//        - "/ws/nats"  → proxy gateway; session cookie auth, no query param.
//        - "/ws"       → agent gateway; still accepts ?token=<bootstrap>.
//   3. The JWT/seed are stashed on the Bus so a future task can append them
//      to the WS handshake; today the agent gateway doesn't read them yet.
//   4. The cred is refreshed at ~half its remaining lifetime (covers both the
//      1h proxy TTL and the 15min agent TTL). 401 → redirect to /auth/login.

import { fetchMe } from './mode';
import { getToken } from './session';

export type SubEnvelope = { action: 'sub'; subject: string };
export type UnsubEnvelope = { action: 'unsub'; subject: string };
export type PubEnvelope = { action: 'pub'; subject: string; data: string };
export type MsgEnvelope = { action: 'msg'; subject: string; data: string };

export type NatsCred = {
  jwt: string;
  seed: string; // base64-encoded NKey seed
  expiresAt: number; // unix seconds
  natsUrl: string; // "/ws" (agent) or "/ws/nats" (proxy)
};

export function encodeSub(subject: string): SubEnvelope {
  return { action: 'sub', subject };
}

export function encodePub(subject: string, data: Uint8Array): PubEnvelope {
  let s = '';
  for (const b of data) s += String.fromCharCode(b);
  return { action: 'pub', subject, data: btoa(s) };
}

export function decodeEnvelope(env: MsgEnvelope): { subject: string; data: Uint8Array } {
  const bin = atob(env.data);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return { subject: env.subject, data: out };
}

/** Fetches a short-lived NATS user credential from the host.
 *
 * The agent gateway accepts either the `wp_local_token` cookie or a `?token=`
 * query param matching the bootstrap admin token. On the trusted LAN the agent
 * auto-issues the cookie when it serves the SPA (it's also set by the
 * magic-link `/login` flow), so `credentials:'include'` usually suffices; we
 * still append `?token=` as an off-LAN fallback — symmetric with `buildWsUrl`.
 * The proxy ignores the extra query param. */
export async function fetchNatsCred(): Promise<NatsCred> {
  const r = await fetch(`/api/auth/nats-cred?token=${encodeURIComponent(getToken())}`, {
    method: 'POST',
    credentials: 'include',
  });
  if (r.status === 401) {
    // Session expired / unauthenticated. The WorkOS login route lives at
    // /auth/login but only exists in proxy mode — the agent's mux doesn't
    // serve that path, so redirecting there from local mode would 404. Local
    // mode authenticates via the wp_local_token cookie (auto-issued on the
    // trusted LAN) or a ?token= query param (see state/session.ts); a 401 here
    // means neither was accepted, and the UI should surface the error rather
    // than blind-redirect.
    const me = await fetchMe();
    if (me.mode === 'local') {
      throw new Error(
        'unauthorized — no local session. Reach the rover directly on the LAN ' +
        '(it auto-authenticates there, e.g. http://waypoint.local/); otherwise ' +
        'append ?token=<bootstrap-admin-token> or use a magic link.',
      );
    }
    const next = window.location.pathname + window.location.search;
    window.location.href = `/auth/login?next=${encodeURIComponent(next)}`;
    throw new Error('unauthorized — redirecting to /auth/login');
  }
  if (!r.ok) throw new Error(`/api/auth/nats-cred → ${r.status}`);
  return (await r.json()) as NatsCred;
}

/** Default builder: pick WS protocol; append ?token only as an agent-mode fallback. */
export function buildWsUrl(cred: NatsCred): string {
  const loc = window.location;
  const proto = loc.protocol === 'https:' ? 'wss:' : 'ws:';
  let path = cred.natsUrl;
  if (path === '/ws') {
    // Agent gateway authenticates via the wp_local_token cookie (auto-issued on
    // the trusted LAN). The ?token= is a harmless fallback/override and is empty
    // when no token was supplied.
    path = `/ws?token=${encodeURIComponent(getToken())}`;
  }
  // Proxy mode ("/ws/nats") relies on the session cookie alone.
  return `${proto}//${loc.host}${path}`;
}

/** Subscriber callback. Receives raw protobuf bytes and the matched subject. */
export type Handler = (data: Uint8Array, subject: string) => void;

/**
 * Returns true when `subject` matches `pattern`, supporting NATS wildcard
 * tokens:
 *   - `*`  matches exactly one token
 *   - `>`  matches one or more trailing tokens (must be the final token in the
 *          pattern)
 */
export function matchesSubject(pattern: string, subject: string): boolean {
  if (pattern === subject) return true;
  const pTok = pattern.split('.');
  const sTok = subject.split('.');
  for (let i = 0; i < pTok.length; i++) {
    if (pTok[i] === '>') return i === pTok.length - 1 && i < sTok.length;
    if (i >= sTok.length) return false;
    if (pTok[i] === '*') continue;
    if (pTok[i] !== sTok[i]) return false;
  }
  return pTok.length === sTok.length;
}

/**
 * Connection to the host's WebSocket bridge. Subscriptions are deduplicated
 * across overlapping patterns: when a wildcard covers an exact subject (or
 * another wildcard), only the broadest pattern is registered with the server.
 * The agent's NATS bridge would otherwise deliver one frame per matching
 * server-side subscription, so a wildcard plus an exact sub on the same
 * concrete subject doubles every counter downstream.
 * Publish is fire-and-forget. The Bus owns the credential lifecycle: it
 * fetches on `start()`, reconnects with a fresh URL on refresh, and reapplies
 * server subs on reconnect.
 */
export class Bus {
  private ws: WebSocket | null = null;
  private subs = new Map<string, Set<Handler>>();
  // Patterns currently registered on the server-side NATS bridge. May be a
  // strict subset of `subs` keys: a covering wildcard suppresses narrower
  // entries.
  private serverSubs = new Set<string>();
  private queue: object[] = [];
  private cred: NatsCred | null = null;
  private refreshTimer: ReturnType<typeof setTimeout> | null = null;

  /**
   * @param buildUrl  given a NatsCred, return the WebSocket URL to dial.
   * @param fetchCred override the credential fetcher (tests).
   */
  constructor(
    private readonly buildUrl: (cred: NatsCred) => string = buildWsUrl,
    private readonly fetchCred: () => Promise<NatsCred> = fetchNatsCred,
  ) {}

  /** Fetch the initial credential and open the WS. Idempotent. */
  async start(): Promise<void> {
    if (this.cred) return;
    this.cred = await this.fetchCred();
    this.scheduleRefresh();
    this.connect();
  }

  /** Exposed for future tasks that want to attach the JWT to the handshake. */
  getCred(): NatsCred | null {
    return this.cred;
  }

  private scheduleRefresh() {
    if (!this.cred) return;
    if (this.refreshTimer != null) clearTimeout(this.refreshTimer);
    const now = Date.now() / 1000;
    const lifeRemaining = this.cred.expiresAt - now;
    // Refresh at ~half remaining lifetime (covers both 1h and 15min TTLs).
    // Floor at 30s so we don't spin during tests with stubbed past timestamps.
    const refreshIn = Math.max(30_000, Math.floor((lifeRemaining * 1000) / 2));
    this.refreshTimer = setTimeout(() => {
      void this.refresh();
    }, refreshIn);
  }

  private async refresh() {
    try {
      this.cred = await this.fetchCred();
      this.scheduleRefresh();
      // Reconnect with the fresh URL — old WS's onopen path will re-subscribe.
      if (this.ws) {
        try {
          this.ws.close();
        } catch {
          /* ignore */
        }
        this.ws = null;
      }
      this.connect();
    } catch {
      // The 401 path is handled in fetchNatsCred (redirect).
      // Network blip etc.: retry in 30s without tearing down the existing WS.
      this.refreshTimer = setTimeout(() => {
        void this.refresh();
      }, 30_000);
    }
  }

  private connect() {
    if (!this.cred) return;
    const ws = new WebSocket(this.buildUrl(this.cred));
    this.ws = ws;
    ws.onopen = () => {
      // Re-subscribe only to the deduped server set; queued envelopes (sub,
      // unsub, pub issued before the socket opened) still need to be flushed
      // in arrival order so unsub never undoes a follow-up sub.
      for (const subj of this.serverSubs) {
        ws.send(JSON.stringify(encodeSub(subj)));
      }
      while (this.queue.length) {
        const env = this.queue.shift();
        if (env) ws.send(JSON.stringify(env));
      }
    };
    ws.onmessage = (e) => {
      try {
        const env = JSON.parse(e.data) as MsgEnvelope;
        if (env.action !== 'msg') return;
        const decoded = decodeEnvelope(env);
        // Exact-match dispatch.
        const hs = this.subs.get(env.subject);
        if (hs) {
          for (const h of hs) h(decoded.data, env.subject);
        }
        // Wildcard dispatch: patterns containing * or > that match env.subject.
        for (const [pattern, handlers] of this.subs) {
          if (pattern === env.subject) continue;          // already dispatched above
          if (!/[*>]/.test(pattern)) continue;            // not a wildcard pattern
          if (!matchesSubject(pattern, env.subject)) continue;
          for (const h of handlers) h(decoded.data, env.subject);
        }
      } catch {
        // ignore malformed frames
      }
    };
    ws.onclose = () => {
      this.ws = null;
      setTimeout(() => this.connect(), 1000);
    };
    ws.onerror = () => {
      // Swallow; onclose will retry.
    };
  }

  subscribe(subject: string, handler: Handler): () => void {
    let set = this.subs.get(subject);
    const firstHandler = !set;
    if (!set) {
      set = new Set();
      this.subs.set(subject, set);
    }
    set.add(handler);
    if (firstHandler) this.addServerSub(subject);
    return () => {
      const s = this.subs.get(subject);
      if (!s) return;
      s.delete(handler);
      if (s.size > 0) return;
      this.subs.delete(subject);
      this.removeServerSub(subject);
    };
  }

  // Register a new pattern with the server, but only when no existing server
  // sub already covers it. If the new pattern itself covers any existing
  // server sub, those narrower entries get unsubscribed first.
  private addServerSub(subject: string) {
    for (const existing of this.serverSubs) {
      if (matchesSubject(existing, subject)) return;
    }
    for (const existing of Array.from(this.serverSubs)) {
      if (existing !== subject && matchesSubject(subject, existing)) {
        this.serverSubs.delete(existing);
        this.send({ action: 'unsub', subject: existing } satisfies UnsubEnvelope);
      }
    }
    this.serverSubs.add(subject);
    this.send(encodeSub(subject));
  }

  // Drop a pattern from the server set. If it was a covering wildcard, any
  // local patterns that lost their coverage need their own server sub now.
  private removeServerSub(subject: string) {
    if (!this.serverSubs.delete(subject)) return;
    this.send({ action: 'unsub', subject } satisfies UnsubEnvelope);
    for (const pattern of this.subs.keys()) {
      if (this.serverSubs.has(pattern)) continue;
      let covered = false;
      for (const s of this.serverSubs) {
        if (matchesSubject(s, pattern)) {
          covered = true;
          break;
        }
      }
      if (!covered) {
        this.serverSubs.add(pattern);
        this.send(encodeSub(pattern));
      }
    }
  }

  publish(subject: string, data: Uint8Array) {
    this.send(encodePub(subject, data));
  }

  private send(env: object) {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(env));
    } else {
      this.queue.push(env);
    }
  }
}

/**
 * Default singleton. Returns synchronously; the WS connect happens after
 * /api/auth/nats-cred resolves. Subscribers/publishes registered before the
 * socket opens are queued by `send()` and flushed on `onopen`.
 */
let _bus: Bus | null = null;
export function getBus(): Bus {
  if (_bus) return _bus;
  _bus = new Bus();
  void _bus.start();
  return _bus;
}
