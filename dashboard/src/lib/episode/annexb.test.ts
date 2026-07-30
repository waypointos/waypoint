import { describe, expect, it } from 'vitest';
import { splitNals, auInfo, codecStringFromSps } from './annexb';

const SPS = [0x67, 0x42, 0x00, 0x1e];
const PPS = [0x68, 0xce];
const IDR = [0x65, 0x88];
const NON_IDR = [0x41, 0x9a];

const cat = (...parts: number[][]) => new Uint8Array(parts.flat());
const sc4 = [0, 0, 0, 1];
const sc3 = [0, 0, 1];

describe('annexb', () => {
  it('splits NALs across 3- and 4-byte start codes', () => {
    const au = cat(sc4, SPS, sc3, PPS, sc4, IDR);
    const nals = splitNals(au);
    expect(nals.map((n) => n[0])).toEqual([0x67, 0x68, 0x65]);
  });

  it('classifies keyframes and extracts SPS/PPS', () => {
    const key = auInfo(cat(sc4, SPS, sc4, PPS, sc4, IDR));
    expect(key.key).toBe(true);
    expect(Array.from(key.sps!)).toEqual(SPS);
    expect(Array.from(key.pps!)).toEqual(PPS);

    const delta = auInfo(cat(sc4, NON_IDR));
    expect(delta.key).toBe(false);
    expect(delta.sps).toBeNull();
  });

  it('derives the avc1 codec string from SPS bytes', () => {
    expect(codecStringFromSps(new Uint8Array(SPS))).toBe('avc1.42001e');
  });
});
