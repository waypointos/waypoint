// dashboard/src/ui/telemetry/HeadingTape.tsx
//
// Scrolling compass strip for the teleop HUD. No IMU/magnetometer is fitted,
// so heading is dead-reckoned by integrating the measured yaw rate. It is
// relative (drifts, resets on reload), hence the REL tag. Draws to its own
// canvas via rAF so it never re-renders the console.
import { useEffect, useRef } from 'react';
import styles from './HeadingTape.module.css';

const CARDINAL: Record<number, string> = {
  0: 'N', 45: 'NE', 90: 'E', 135: 'SE', 180: 'S', 225: 'SW', 270: 'W', 315: 'NW',
};

export function HeadingTape({ yawRateRadps }: { yawRateRadps: number | null | undefined }) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const yawRef = useRef(0);
  yawRef.current = yawRateRadps ?? 0;
  const headingRef = useRef(0);

  useEffect(() => {
    const canvas = canvasRef.current;
    const ctx = canvas?.getContext('2d');
    if (!canvas || !ctx) return;
    const dpr = window.devicePixelRatio || 1;
    const W = 300, H = 30;
    canvas.width = W * dpr;
    canvas.height = H * dpr;

    const cs = getComputedStyle(canvas);
    const fg2 = cs.getPropertyValue('--color-fg-2').trim();
    const fg4 = cs.getPropertyValue('--color-fg-4').trim();
    const accent = cs.getPropertyValue('--color-accent').trim();
    const mono = cs.getPropertyValue('--font-mono').trim() || 'monospace';

    let raf = 0;
    let last = performance.now();
    const draw = (now: number) => {
      const dt = (now - last) / 1000;
      last = now;
      // Integrate yaw rate into a relative heading (right turn = heading up).
      headingRef.current = ((headingRef.current - (yawRef.current * dt * 180) / Math.PI) % 360 + 360) % 360;
      const heading = headingRef.current;

      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      ctx.clearRect(0, 0, W, H);
      const pxPerDeg = 3;
      ctx.font = `8px ${mono}`;
      ctx.textAlign = 'center';
      for (let d = -50; d <= 50; d += 5) {
        const hdg = ((Math.round(heading / 5) * 5 + d) % 360 + 360) % 360;
        const delta = ((hdg - heading + 540) % 360) - 180;
        const x = 150 + delta * pxPerDeg;
        if (x < 4 || x > 296) continue;
        const major = hdg % 15 === 0;
        ctx.strokeStyle = fg4;
        ctx.lineWidth = 1;
        ctx.beginPath();
        ctx.moveTo(x, 20);
        ctx.lineTo(x, major ? 12 : 16);
        ctx.stroke();
        if (major) {
          ctx.fillStyle = CARDINAL[hdg] ? fg2 : fg4;
          ctx.fillText(CARDINAL[hdg] ?? String(hdg).padStart(3, '0'), x, 9);
        }
      }
      // Centre pointer.
      ctx.strokeStyle = accent;
      ctx.lineWidth = 1.4;
      ctx.beginPath();
      ctx.moveTo(150, 22);
      ctx.lineTo(147, 28);
      ctx.lineTo(153, 28);
      ctx.closePath();
      ctx.stroke();

      raf = requestAnimationFrame(draw);
    };
    raf = requestAnimationFrame(draw);
    return () => cancelAnimationFrame(raf);
  }, []);

  return (
    <div className={styles.tapeWrap} aria-hidden>
      <canvas ref={canvasRef} className={styles.tape} />
      <span className={styles.tapeTag}>rel</span>
    </div>
  );
}
