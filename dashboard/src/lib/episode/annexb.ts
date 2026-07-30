// dashboard/src/lib/episode/annexb.ts
//
// Minimal H.264 Annex-B helpers for WebCodecs configuration and keyframe
// detection. NAL type is the low 5 bits of the first payload byte.
export function splitNals(au: Uint8Array): Uint8Array[] {
  const starts: number[] = [];
  for (let i = 0; i + 2 < au.length; i++) {
    if (au[i] === 0 && au[i + 1] === 0 && au[i + 2] === 1) {
      starts.push(i + 3);
      i += 2;
    }
  }
  return starts.map((s, idx) => {
    let end = idx + 1 < starts.length ? starts[idx + 1] - 3 : au.length;
    // A 4-byte start code leaves a stray 0 before the next 001.
    if (end > s && au[end - 1] === 0 && idx + 1 < starts.length) end--;
    return au.subarray(s, end);
  });
}

export function auInfo(au: Uint8Array): { key: boolean; sps: Uint8Array | null; pps: Uint8Array | null } {
  let key = false;
  let sps: Uint8Array | null = null;
  let pps: Uint8Array | null = null;
  for (const nal of splitNals(au)) {
    const type = nal[0] & 0x1f;
    if (type === 5) key = true;
    if (type === 7) sps = nal;
    if (type === 8) pps = nal;
  }
  return { key, sps, pps };
}

export function codecStringFromSps(sps: Uint8Array): string {
  const hex = (b: number) => b.toString(16).padStart(2, '0');
  return `avc1.${hex(sps[1])}${hex(sps[2])}${hex(sps[3])}`;
}
