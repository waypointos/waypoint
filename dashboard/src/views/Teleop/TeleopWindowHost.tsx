import { useEffect, useRef, useState } from 'react';
import { tokens } from '@waypoint/ui-tokens';
import { getBus } from '@/state/nats';
import { useMe } from '@/state/mode';
import { useDragResize } from '@/ui/primitives/useDragResize';
import { moduleStaticUrl, waypointImport } from '@/lib/moduleImport';
import type { TeleopWindowDef } from './teleopWindows';
import styles from './TeleopWindowHost.module.css';

type Bundle = { default: { mount(el: HTMLElement, ctx: unknown): () => void } };

function WindowFrame({ roverId, win, onClose }: { roverId: string; win: TeleopWindowDef; onClose: () => void }) {
  const hostRef = useRef<HTMLDivElement>(null);
  const bodyRef = useRef<HTMLDivElement>(null);
  const me = useMe();
  const { pos, size, startMove, startResize } = useDragResize({
    initialPosition: { x: 80, y: 80 },
    initialSize: { width: 620, height: 460 },
    minSize: { width: 320, height: 260 },
  });

  useEffect(() => {
    const el = bodyRef.current;
    if (!el) return;
    let cleanup: (() => void) | undefined;
    let cancelled = false;
    moduleStaticUrl(me?.mode, roverId, win.moduleId, win.entry)
      .then((url) => waypointImport<Bundle>(url))
      .then((mod) => {
        if (cancelled) return;
        cleanup = mod.default.mount(el, {
          roverId,
          tokens,
          subscribe: (subject: string, onBytes: (b: Uint8Array) => void) => getBus().subscribe(subject, onBytes),
          publish: (subject: string, bytes: Uint8Array) => {
            const prefix = `waypoint.${roverId}.module.${win.moduleId}.`;
            if (!subject.startsWith(prefix)) throw new Error(`module ${win.moduleId} may not publish to ${subject}`);
            getBus().publish(subject, bytes);
          },
        });
      })
      .catch((err) => {
        if (!cancelled) el.textContent = `Failed to load module: ${err?.message ?? err}`;
      });
    return () => { cancelled = true; if (cleanup) cleanup(); };
  }, [roverId, win.moduleId, win.entry, me?.mode]);

  return (
    <div
      ref={hostRef}
      className={styles.window}
      style={{ left: pos.x, top: pos.y, width: size.width, height: size.height }}
      data-teleop-window={win.windowId}
    >
      <div className={styles.bar} onPointerDown={startMove}>
        <span className={styles.title}>{win.label}</span>
        <button type="button" className={styles.close} onClick={onClose} aria-label={`Close ${win.label}`}>✕</button>
      </div>
      <div ref={bodyRef} className={styles.body} />
      <div className={styles.resize} onPointerDown={startResize} />
    </div>
  );
}

export function TeleopWindowHost({ roverId, windows }: { roverId: string; windows: TeleopWindowDef[] }) {
  const [open, setOpen] = useState<Record<string, boolean>>({});
  return (
    <>
      <div className={styles.dock}>
        {windows.map((w) => (
          <button
            key={w.windowId}
            type="button"
            className={open[w.windowId] ? `${styles.chip} ${styles.chipOn}` : styles.chip}
            aria-pressed={!!open[w.windowId]}
            onClick={() => setOpen((o) => ({ ...o, [w.windowId]: !o[w.windowId] }))}
          >
            {w.label}
          </button>
        ))}
      </div>
      {windows.filter((w) => open[w.windowId]).map((w) => (
        <WindowFrame key={w.windowId} roverId={roverId} win={w} onClose={() => setOpen((o) => ({ ...o, [w.windowId]: false }))} />
      ))}
    </>
  );
}
