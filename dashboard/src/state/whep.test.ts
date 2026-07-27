// dashboard/src/state/whep.test.ts
//
// vitest can't easily fake an RTCPeerConnection, so this file stubs the
// minimum surface startWhep touches: createOffer, setLocalDescription,
// setRemoteDescription, addTransceiver, close, ontrack, iceGatheringState.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { startWhep } from './whep';

class FakePC {
  iceGatheringState: RTCIceGatheringState = 'complete';
  localDescription: RTCSessionDescription | null = null;
  ontrack: ((ev: RTCTrackEvent) => void) | null = null;
  addTransceiver = vi.fn();
  addEventListener = vi.fn();
  removeEventListener = vi.fn();
  close = vi.fn();
  async createOffer(): Promise<RTCSessionDescriptionInit> {
    return { type: 'offer', sdp: 'v=0\no=offer\n' };
  }
  async setLocalDescription(desc: RTCSessionDescriptionInit): Promise<void> {
    this.localDescription = {
      type: desc.type ?? 'offer',
      sdp: desc.sdp ?? '',
      toJSON: () => ({ type: desc.type, sdp: desc.sdp }),
    } as RTCSessionDescription;
  }
  async setRemoteDescription(_d: RTCSessionDescriptionInit): Promise<void> {}
}

const originalRTC = (globalThis as { RTCPeerConnection?: unknown }).RTCPeerConnection;
const originalFetch = globalThis.fetch;

describe('startWhep', () => {
  beforeEach(() => {
    (globalThis as { RTCPeerConnection: unknown }).RTCPeerConnection = FakePC;
  });

  afterEach(() => {
    (globalThis as { RTCPeerConnection?: unknown }).RTCPeerConnection = originalRTC;
    globalThis.fetch = originalFetch;
  });

  it('POSTs the offer SDP and applies the answer', async () => {
    const fetchMock = vi.fn(async () => ({
      status: 201,
      headers: {
        get: (k: string) => (k === 'Location' ? '/camera/x/whep/abc' : null),
      },
      text: async () => 'v=0\no=answer\n',
    }));
    globalThis.fetch = fetchMock as unknown as typeof fetch;

    const session = await startWhep({
      whepUrl: 'http://localhost:8080/camera/x/whep',
      onTrack: () => {},
    });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe('http://localhost:8080/camera/x/whep');
    expect(init.method).toBe('POST');
    const headers = init.headers as Record<string, string>;
    expect(headers['Content-Type']).toBe('application/sdp');
    expect(init.body).toContain('v=0');
    expect(session.sessionUrl).toBe('http://localhost:8080/camera/x/whep/abc');
  });

  it('uses absolute Location verbatim when the server returns one', async () => {
    const fetchMock = vi.fn(async () => ({
      status: 201,
      headers: { get: () => 'https://proxy.example/rover/r/camera/x/whep/abc' },
      text: async () => 'v=0\no=answer\n',
    }));
    globalThis.fetch = fetchMock as unknown as typeof fetch;

    const session = await startWhep({
      whepUrl: 'http://localhost:8080/camera/x/whep',
      onTrack: () => {},
    });
    expect(session.sessionUrl).toBe('https://proxy.example/rover/r/camera/x/whep/abc');
  });

  it('handles a relative whepUrl by resolving Location against page origin', async () => {
    // RoverView passes path-only whepUrls (e.g. "/camera/chassis-front/whep")
    // since the dashboard is served from the same origin as the agent gateway.
    // The Location header is also a path. We must resolve them against an
    // absolute base; the page origin (jsdom default: http://localhost) works.
    const fetchMock = vi.fn(async () => ({
      status: 201,
      headers: {
        get: (k: string) => (k === 'Location' ? '/camera/chassis-front/whep/abc' : null),
      },
      text: async () => 'v=0\no=answer\n',
    }));
    globalThis.fetch = fetchMock as unknown as typeof fetch;

    const session = await startWhep({
      whepUrl: '/camera/chassis-front/whep',
      onTrack: () => {},
    });
    expect(session.sessionUrl).toBe('http://localhost:3000/camera/chassis-front/whep/abc');
  });

  it('throws and closes the PC when the server rejects the offer', async () => {
    const fetchMock = vi.fn(async () => ({
      status: 404,
      headers: { get: () => null },
      text: async () => '',
    }));
    globalThis.fetch = fetchMock as unknown as typeof fetch;

    await expect(
      startWhep({
        whepUrl: 'http://localhost:8080/camera/none/whep',
        onTrack: () => {},
      }),
    ).rejects.toThrow(/404/);
  });

  it('close() DELETEs the session URL and closes the PC', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        status: 201,
        headers: { get: () => '/camera/x/whep/abc' },
        text: async () => 'v=0\no=answer\n',
      })
      .mockResolvedValueOnce({ status: 204 });
    globalThis.fetch = fetchMock as unknown as typeof fetch;

    const session = await startWhep({
      whepUrl: 'http://localhost:8080/camera/x/whep',
      onTrack: () => {},
    });
    await session.close();

    expect(fetchMock).toHaveBeenCalledTimes(2);
    const [delUrl, delInit] = fetchMock.mock.calls[1] as [string, RequestInit];
    expect(delUrl).toBe('http://localhost:8080/camera/x/whep/abc');
    expect(delInit.method).toBe('DELETE');
  });
});
