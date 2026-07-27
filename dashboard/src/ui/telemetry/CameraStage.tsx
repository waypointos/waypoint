// A main CameraView plus optional clickable picture-in-picture overlays.
// Clicking (or Enter/Space on) a PiP promotes it to the main slot.
// All streams stay mounted across swaps so the WHEP sessions don't restart.
//
// floatingPip turns each PiP into a draggable, corner-resizable window
// (Teleop). A small movement threshold separates drag from click-to-swap.
import { useRef, useState, type ReactNode } from 'react';

import { useDragResize } from '@/ui/primitives/useDragResize';
import { CameraView } from './CameraView';
import styles from './CameraStage.module.css';

export interface CameraStageStream {
  name: string;
  whepUrl: string;
}

export interface CameraStageProps {
  streams: CameraStageStream[];
  iceServers?: RTCIceServer[];
  /** Render PiPs as draggable/resizable floating windows (Teleop HUD). */
  floatingPip?: boolean;
  /**
   * Show the main feed's built-in HUD (bracket corners, reticle, label/flip
   * strip). Teleop draws its own HUD over the feed, so it sets this false to
   * avoid a duplicate reticle and chrome.
   */
  mainHudShown?: boolean;
}

const PIP_SIZE = { width: 256, height: 148 };
const PIP_MIN = { width: 180, height: 104 };
const DRAG_THRESHOLD_PX = 4;

function FloatingPip({ label, onSwap, children }: {
  label: string;
  onSwap: () => void;
  children: ReactNode;
}) {
  const { pos, size, startMove, startResize } = useDragResize({
    initialPosition: {
      x: Math.max(0, window.innerWidth - PIP_SIZE.width - 16),
      y: Math.max(0, window.innerHeight - PIP_SIZE.height - 128),
    },
    initialSize: PIP_SIZE,
    minSize: PIP_MIN,
  });
  const down = useRef<{ x: number; y: number; dragging: boolean } | null>(null);

  return (
    <div
      className={styles.pipFloat}
      style={{ left: pos.x, top: pos.y, width: size.width, height: size.height }}
      role="button"
      tabIndex={0}
      aria-label={`Switch to ${label} camera`}
      data-floating-pip
      onPointerDown={(e) => {
        down.current = { x: e.clientX, y: e.clientY, dragging: false };
      }}
      onPointerMove={(e) => {
        const d = down.current;
        if (!d || d.dragging) return;
        if (Math.hypot(e.clientX - d.x, e.clientY - d.y) > DRAG_THRESHOLD_PX) {
          d.dragging = true;
          startMove({ clientX: d.x, clientY: d.y } as React.PointerEvent);
        }
      }}
      onPointerUp={() => {
        const d = down.current;
        down.current = null;
        if (d && !d.dragging) onSwap();
      }}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          onSwap();
        }
      }}
    >
      {children}
      <span
        className={styles.pipResize}
        aria-hidden
        onPointerDown={(e) => {
          e.stopPropagation();
          startResize(e);
        }}
      />
    </div>
  );
}

export function CameraStage({ streams, iceServers, floatingPip = false, mainHudShown = true }: CameraStageProps) {
  const [activeName, setActiveName] = useState<string | undefined>(streams[0]?.name);

  if (streams.length === 0) return null;

  // Clamp to a valid stream if the active one disappears (e.g. rover swap).
  const active = streams.find((s) => s.name === activeName) ?? streams[0];

  return (
    <div className={styles.stage} data-testid="camera-stage">
      {streams.map((stream) => {
        const isActive = stream.name === active.name;
        if (isActive) {
          return (
            <CameraView
              key={stream.name}
              whepUrl={stream.whepUrl}
              label={stream.name}
              iceServers={iceServers}
              hudShown={mainHudShown}
            />
          );
        }
        const view = (
          <>
            <CameraView
              whepUrl={stream.whepUrl}
              label={stream.name}
              iceServers={iceServers}
              hudShown={false}
            />
            <span className={styles.pipLabel}>{stream.name}</span>
          </>
        );
        if (floatingPip) {
          return (
            <FloatingPip key={stream.name} label={stream.name} onSwap={() => setActiveName(stream.name)}>
              {view}
            </FloatingPip>
          );
        }
        return (
          <div
            key={stream.name}
            className={styles.pip}
            role="button"
            tabIndex={0}
            aria-label={`Switch to ${stream.name} camera`}
            onClick={() => setActiveName(stream.name)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                setActiveName(stream.name);
              }
            }}
          >
            {view}
          </div>
        );
      })}
    </div>
  );
}
