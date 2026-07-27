// Host / image overview. Surfaces the live SystemTelemetry stream as
// sparklines (CPU, memory, temperature, link RTT), plus uptime and the
// ImageState heartbeat (version, partition, bootcount).
//
// History buffers are bounded at HISTORY_LEN samples (~120s at the 1 Hz
// SystemTelemetry cadence). Sparkline samples are pushed only when the
// underlying telemetry reference changes, not on every panel re-render.
import { useEffect, useRef, useState } from 'react';
import { Panel } from '@/ui/primitives/Panel';
import { Metric } from '@/ui/data/Metric';
import { MetricGrid } from '@/ui/data/MetricGrid';
import { Sparkline } from '@/ui/data/Sparkline';
import { useRoverContext } from '../RoverContext';
import styles from './SystemPanel.module.css';

const HISTORY_LEN = 120;

type Series = ReadonlyArray<number | null>;

function pushSample(prev: Series, sample: number | null): Series {
  const next = prev.length >= HISTORY_LEN ? prev.slice(1) : prev.slice();
  next.push(sample);
  return next;
}

export function SystemPanel() {
  const ctx = useRoverContext();

  const [cpu,  setCpu]  = useState<Series>([]);
  const [mem,  setMem]  = useState<Series>([]);
  const [temp, setTemp] = useState<Series>([]);
  const [rtt,  setRtt]  = useState<Series>([]);

  // Record one sample per *new* telemetry message. The shell rebuilds ctx
  // on every render, but the underlying telemetry ref only changes when a
  // new SystemTelemetry arrives — so depending on ctx.sys (not ctx) keeps
  // us at the real 1 Hz cadence.
  const lastSysRef = useRef<typeof ctx.sys>(null);
  useEffect(() => {
    if (ctx.sys === lastSysRef.current) return;
    lastSysRef.current = ctx.sys;
    setCpu( (p) => pushSample(p, ctx.sys?.cpuPercent     ?? null));
    setTemp((p) => pushSample(p, ctx.sys?.temperatureC   ?? null));
    setRtt( (p) => pushSample(p, ctx.sys?.linkRttMs      ?? null));
    setMem( (p) => pushSample(p, memPctFromSys(ctx.sys)));
  }, [ctx.sys]);

  const memTotal = numericOr(ctx.sys?.memoryTotalBytes);
  const memUsed  = numericOr(ctx.sys?.memoryUsedBytes);
  const memPct   = memPctFromSys(ctx.sys);

  return (
    <div className={styles.layout} data-testid="panel-system">
      <Panel title="HOST" note="live · 1 hz">
        <div className={styles.row}>
          <div className={styles.cell}>
            <div className={styles.cellHead}>
              <span className={styles.cellLbl}>CPU</span>
              <span className={styles.cellVal}>
                {ctx.sys?.cpuPercent == null ? '—' : `${ctx.sys.cpuPercent.toFixed(0)}%`}
              </span>
            </div>
            <Sparkline values={cpu} min={0} max={100} filled />
          </div>
          <div className={styles.cell}>
            <div className={styles.cellHead}>
              <span className={styles.cellLbl}>Memory</span>
              <span className={styles.cellVal}>
                {memPct == null ? '—' : `${memPct.toFixed(0)}%`}
              </span>
            </div>
            <Sparkline values={mem} min={0} max={100} filled />
            <div className={styles.cellSub}>
              {memUsed == null || memTotal == null
                ? 'memory probe N/A'
                : `${humanBytes(memUsed)} / ${humanBytes(memTotal)}`}
            </div>
          </div>
          <div className={styles.cell}>
            <div className={styles.cellHead}>
              <span className={styles.cellLbl}>Temp</span>
              <span className={styles.cellVal}>
                {ctx.sys?.temperatureC == null ? '—' : `${ctx.sys.temperatureC.toFixed(0)} °C`}
              </span>
            </div>
            <Sparkline values={temp} stroke="var(--color-caution)" filled />
          </div>
          <div className={styles.cell}>
            <div className={styles.cellHead}>
              <span className={styles.cellLbl}>Link RTT</span>
              <span className={styles.cellVal}>
                {ctx.sys?.linkRttMs == null ? '—' : `${ctx.sys.linkRttMs.toFixed(0)} ms`}
              </span>
            </div>
            <Sparkline values={rtt} filled />
          </div>
        </div>
      </Panel>

      <Panel title="IMAGE" note="infra · system.image">
        <MetricGrid>
          <Metric label="Version"   value={ctx.image?.version    || ctx.sys?.imageVersion   || null} naReason="no image heartbeat" tone="key" />
          <Metric label="Partition" value={ctx.image?.partition  || ctx.sys?.imagePartition || null} naReason="no image heartbeat" />
          <Metric label="Variant"   value={ctx.image?.variant    || null} naReason="no image heartbeat" />
          <Metric label="Boot #"    value={ctx.image?.bootcount  ?? null} naReason="no image heartbeat" />
          <Metric label="Built"     value={ctx.image?.builtAt    || null} naReason="no image heartbeat" />
          <Metric label="Uptime"    value={formatUptime(ctx.sys?.uptimeSeconds)} naReason="no telemetry" />
        </MetricGrid>
      </Panel>
    </div>
  );
}

function numericOr(v: number | bigint | undefined | null): number | null {
  if (v == null) return null;
  return typeof v === 'bigint' ? Number(v) : v;
}

function memPctFromSys(sys: { memoryUsedBytes?: number | bigint; memoryTotalBytes?: number | bigint } | null): number | null {
  const used  = numericOr(sys?.memoryUsedBytes);
  const total = numericOr(sys?.memoryTotalBytes);
  if (used == null || total == null || total === 0) return null;
  return (used / total) * 100;
}

function humanBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KiB`;
  if (n < 1024 * 1024 * 1024) return `${(n / 1024 / 1024).toFixed(1)} MiB`;
  return `${(n / 1024 / 1024 / 1024).toFixed(2)} GiB`;
}

function formatUptime(s: number | bigint | undefined | null): string | null {
  const n = numericOr(s);
  if (n == null) return null;
  const days  = Math.floor(n / 86400);
  const hours = Math.floor((n % 86400) / 3600);
  const mins  = Math.floor((n % 3600) / 60);
  if (days > 0)  return `${days}d ${hours}h ${mins}m`;
  if (hours > 0) return `${hours}h ${mins}m`;
  return `${mins}m`;
}
