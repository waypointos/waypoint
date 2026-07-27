// dashboard/src/ui/telemetry/CameraView.tsx
//
// Live H.264 video tile fed by a WHEP session. The frame is decorated with
// the standard mission-console bracket corners, a centered amber reticle,
// and a bottom strip carrying the camera label.
import { useEffect, useRef, useState } from 'react';
import { FlipVertical } from 'lucide-react';

import { BracketCorners } from '../primitives/BracketCorners';
import { startWhep, type WhepSession } from '../../state/whep';
import styles from './CameraView.module.css';

// Per-camera invert (rotate 180°) is remembered across reloads, keyed by
// camera name — a camera's mount orientation is fixed, so it's set once.
const invertStorageKey = (label: string) => `wp.camera.invert.${label}`;

export interface CameraViewProps {
  whepUrl: string;
  label: string;
  /** Hide the HUD chrome (used by the gripper PiP). Default: true. */
  hudShown?: boolean;
  iceServers?: RTCIceServer[];
}

type Status = 'connecting' | 'live' | 'error';

export function CameraView({
  whepUrl,
  label,
  hudShown = true,
  iceServers,
}: CameraViewProps) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const sessionRef = useRef<WhepSession | null>(null);
  const [status, setStatus] = useState<Status>('connecting');
  const [inverted, setInverted] = useState<boolean>(() => {
    try {
      return localStorage.getItem(invertStorageKey(label)) === '1';
    } catch {
      return false;
    }
  });

  useEffect(() => {
    try {
      localStorage.setItem(invertStorageKey(label), inverted ? '1' : '0');
    } catch {
      /* localStorage unavailable (e.g. private mode): invert just won't persist */
    }
  }, [label, inverted]);

  useEffect(() => {
    let cancelled = false;
    setStatus('connecting');
    (async () => {
      try {
        const sess = await startWhep({
          whepUrl,
          iceServers,
          onTrack: (stream) => {
            if (videoRef.current) videoRef.current.srcObject = stream;
            setStatus('live');
          },
        });
        if (cancelled) {
          await sess.close();
          return;
        }
        sessionRef.current = sess;
      } catch (err) {
        if (!cancelled) {
          console.error('WHEP error', err);
          setStatus('error');
        }
      }
    })();
    return () => {
      cancelled = true;
      sessionRef.current?.close();
      sessionRef.current = null;
    };
  }, [whepUrl, iceServers]);

  return (
    <div className={styles.frame} data-status={status}>
      <video
        ref={videoRef}
        className={styles.video}
        data-inverted={inverted}
        autoPlay
        playsInline
        muted
      />
      {status !== 'live' && (
        <div className={styles.overlay}>
          {status === 'connecting' ? 'connecting…' : 'N/A — stream unavailable'}
        </div>
      )}
      {hudShown && (
        <>
          <BracketCorners full size={14} />
          <div className={styles.reticle} aria-hidden />
          <div className={styles.bottomStrip}>
            <span className={styles.label}>{label}</span>
            <div className={styles.controls}>
              <button
                type="button"
                className={styles.flip}
                data-active={inverted}
                aria-pressed={inverted}
                aria-label={`Invert ${label} feed (rotate 180°)`}
                title="Invert (rotate 180°)"
                onClick={() => setInverted((v) => !v)}
              >
                <FlipVertical size={13} aria-hidden />
              </button>
              <span className={styles.status} data-status={status}>
                {status === 'live' ? '● live' : status === 'connecting' ? '○ link' : '× n/a'}
              </span>
            </div>
          </div>
        </>
      )}
    </div>
  );
}
