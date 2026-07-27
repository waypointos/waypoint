// dashboard/src/ui/telemetry/JoystickPad.tsx
//
// Virtual joystick. Vertical = forward velocity (vx in -1..1, up = forward).
// Horizontal = yaw rate (yaw in -1..1, right = clockwise / right turn).
// Snaps back to (0, 0) on pointer release.
import { useCallback, useEffect, useRef, useState } from 'react';
import styles from './JoystickPad.module.css';

type Vec = { vx: number; yaw: number };
const ZERO: Vec = { vx: 0, yaw: 0 };

type Props = {
  onChange: (v: Vec) => void;
  disabled?: boolean;
  disabledReason?: string;
};

export function JoystickPad({ onChange, disabled = false, disabledReason }: Props) {
  const ref = useRef<HTMLDivElement>(null);
  const [knob, setKnob] = useState({ x: 0, y: 0 });
  const [active, setActive] = useState(false);

  // Track current values in a ref so the effect cleanup can emit ZERO without stale closure.
  const valueRef = useRef<Vec>(ZERO);
  useEffect(() => () => onChange(ZERO), [onChange]);

  // Fading line-trace of recent stick positions, drawn straight to a canvas so
  // the trail keeps collapsing to centre without re-rendering on every frame.
  const traceRef = useRef<HTMLCanvasElement>(null);
  const trailRef = useRef<{ x: number; y: number }[]>([]);
  useEffect(() => {
    const canvas = traceRef.current;
    const c = canvas?.getContext('2d');
    if (!canvas || !c) return;
    const dpr = window.devicePixelRatio || 1;
    const size = canvas.clientWidth || 200;
    canvas.width = size * dpr;
    canvas.height = size * dpr;
    const accent = getComputedStyle(canvas).getPropertyValue('--color-accent').trim();
    const center = size / 2;
    const rad = size / 2 - 12;

    let raf = 0;
    const draw = () => {
      const trail = trailRef.current;
      trail.push({ x: valueRef.current.yaw, y: -valueRef.current.vx });
      if (trail.length > 40) trail.shift();
      c.setTransform(dpr, 0, 0, dpr, 0, 0);
      c.clearRect(0, 0, size, size);
      c.beginPath();
      trail.forEach((p, i) => {
        const sx = center + p.x * rad;
        const sy = center + p.y * rad;
        if (i === 0) c.moveTo(sx, sy);
        else c.lineTo(sx, sy);
      });
      c.globalAlpha = 0.35;
      c.strokeStyle = accent;
      c.lineWidth = 1;
      c.stroke();
      c.globalAlpha = 1;
      raf = requestAnimationFrame(draw);
    };
    raf = requestAnimationFrame(draw);
    return () => cancelAnimationFrame(raf);
  }, []);

  // If the pad gets disabled mid-interaction, snap back to neutral so we don't
  // leave a hot stick value publishing.
  useEffect(() => {
    if (disabled) {
      setActive(false);
      setKnob({ x: 0, y: 0 });
      valueRef.current = ZERO;
      onChange(ZERO);
    }
  }, [disabled, onChange]);

  const update = useCallback((clientX: number, clientY: number) => {
    const el = ref.current;
    if (!el) return;
    const rect = el.getBoundingClientRect();
    const cx = rect.left + rect.width / 2;
    const cy = rect.top + rect.height / 2;
    const r  = Math.min(rect.width, rect.height) / 2 - 22;
    const dx = clientX - cx;
    const dy = clientY - cy;
    const dist = Math.hypot(dx, dy);
    const scale = dist > r ? r / dist : 1;
    const x = dx * scale;
    const y = dy * scale;
    setKnob({ x, y });
    const v: Vec = { vx: -y / r, yaw: x / r };
    valueRef.current = v;
    onChange(v);
  }, [onChange]);

  const reset = useCallback(() => {
    setKnob({ x: 0, y: 0 });
    valueRef.current = ZERO;
    onChange(ZERO);
  }, [onChange]);

  return (
    <div
      ref={ref}
      data-joy
      data-disabled={disabled || undefined}
      className={`${styles.pad} ${disabled ? styles.disabled : ''}`}
      onPointerDown={(e) => {
        if (disabled) return;
        const t = e.target as HTMLElement & { setPointerCapture?: (id: number) => void };
        t.setPointerCapture?.(e.pointerId);
        setActive(true);
        update(e.clientX, e.clientY);
      }}
      onPointerMove={(e) => !disabled && active && update(e.clientX, e.clientY)}
      onPointerUp={(e) => {
        if (disabled) return;
        const t = e.target as HTMLElement & { releasePointerCapture?: (id: number) => void };
        t.releasePointerCapture?.(e.pointerId);
        setActive(false);
        reset();
      }}
      onPointerCancel={() => {
        if (disabled) return;
        setActive(false);
        reset();
      }}
    >
      <div className={styles.ring} />
      <canvas ref={traceRef} className={styles.trace} aria-hidden />
      <div className={styles.dead} />
      <div className={`${styles.lab} ${styles.labTop}`}>fwd</div>
      <div className={`${styles.lab} ${styles.labBot}`}>rev</div>
      <div className={`${styles.lab} ${styles.labLeft}`}>L</div>
      <div className={`${styles.lab} ${styles.labRight}`}>R</div>
      <div className={styles.knob} style={{ transform: `translate(calc(-50% + ${knob.x}px), calc(-50% + ${knob.y}px))` }} />
      {disabled && disabledReason && <div className={styles.disabledHint}>{disabledReason}</div>}
    </div>
  );
}
