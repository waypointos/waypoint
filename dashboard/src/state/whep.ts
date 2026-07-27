// dashboard/src/state/whep.ts
//
// Minimal WHEP client. Sends the local SDP offer over POST and registers
// the answer; `close` releases the session via DELETE to the URL the server
// returned in the Location header.

export interface WhepSession {
  pc: RTCPeerConnection;
  sessionUrl: string;
  close: () => Promise<void>;
}

export interface WhepOptions {
  whepUrl: string;
  iceServers?: RTCIceServer[];
  onTrack: (stream: MediaStream) => void;
}

export async function startWhep(opts: WhepOptions): Promise<WhepSession> {
  const pc = new RTCPeerConnection({ iceServers: opts.iceServers });
  pc.addTransceiver('video', { direction: 'recvonly' });
  pc.ontrack = (ev) => {
    if (ev.streams[0]) opts.onTrack(ev.streams[0]);
  };

  const offer = await pc.createOffer();
  await pc.setLocalDescription(offer);
  await waitForIceGatheringComplete(pc);

  const resp = await fetch(opts.whepUrl, {
    method: 'POST',
    headers: { 'Content-Type': 'application/sdp' },
    body: pc.localDescription!.sdp,
    credentials: 'include',
  });
  if (resp.status !== 201) {
    pc.close();
    throw new Error(`WHEP POST ${opts.whepUrl} → ${resp.status}`);
  }
  const answer = await resp.text();
  await pc.setRemoteDescription({ type: 'answer', sdp: answer });

  const loc = resp.headers.get('Location') || '';
  // The URL constructor's base must be absolute. opts.whepUrl is usually
  // a path (e.g. "/camera/chassis-front/whep") because RoverView serves
  // the dashboard same-origin as the agent. Resolve against the page
  // origin when whepUrl isn't absolute; fall through to whepUrl when it
  // is (proxy mode, agent URL routed under a /rover/{id} prefix).
  const base = opts.whepUrl.startsWith('http')
    ? opts.whepUrl
    : window.location.origin;
  const sessionUrl = loc.startsWith('http')
    ? loc
    : new URL(loc, base).toString();

  return {
    pc,
    sessionUrl,
    close: async () => {
      try {
        await fetch(sessionUrl, { method: 'DELETE', credentials: 'include' });
      } catch {
        // best-effort teardown; the server eventually times the session out
      }
      pc.close();
    },
  };
}

function waitForIceGatheringComplete(pc: RTCPeerConnection): Promise<void> {
  return new Promise((resolve) => {
    if (pc.iceGatheringState === 'complete') {
      resolve();
      return;
    }
    const handler = () => {
      if (pc.iceGatheringState === 'complete') {
        pc.removeEventListener('icegatheringstatechange', handler);
        resolve();
      }
    };
    pc.addEventListener('icegatheringstatechange', handler);
  });
}
