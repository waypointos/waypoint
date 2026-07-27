import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { CameraView } from './CameraView';

// CameraView starts a WHEP session on mount; in jsdom RTCPeerConnection is
// absent, so startWhep throws and is caught (status → error). The HUD still
// renders, which is all these tests touch. localStorage is stubbed so the
// invert-persistence assertions don't depend on the test runner's storage.

describe('CameraView invert toggle', () => {
  beforeEach(() => {
    const store = new Map<string, string>();
    vi.stubGlobal('localStorage', {
      getItem: (k: string) => (store.has(k) ? store.get(k)! : null),
      setItem: (k: string, v: string) => void store.set(k, String(v)),
      removeItem: (k: string) => void store.delete(k),
      clear: () => store.clear(),
    });
  });
  afterEach(() => vi.unstubAllGlobals());

  it('toggles invert and persists per camera', () => {
    const { container } = render(<CameraView whepUrl="http://x/whep" label="camera-0" />);
    const video = container.querySelector('video')!;
    const btn = screen.getByRole('button', { name: /invert camera-0/i });

    expect(video.getAttribute('data-inverted')).toBe('false');
    fireEvent.click(btn);
    expect(video.getAttribute('data-inverted')).toBe('true');
    expect(localStorage.getItem('wp.camera.invert.camera-0')).toBe('1');

    fireEvent.click(btn);
    expect(video.getAttribute('data-inverted')).toBe('false');
    expect(localStorage.getItem('wp.camera.invert.camera-0')).toBe('0');
  });

  it('restores a persisted invert on mount, per camera name', () => {
    localStorage.setItem('wp.camera.invert.camera-1', '1');
    const { container } = render(<CameraView whepUrl="http://x/whep" label="camera-1" />);
    expect(container.querySelector('video')!.getAttribute('data-inverted')).toBe('true');
  });
});
