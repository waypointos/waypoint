// Read-only "what's happening" view. Cameras + every telemetry rail card.
// No joystick or e-stop — that's Control's job.
import { useState } from 'react';
import { Button } from '@/ui/primitives/Button';
import { Panel } from '@/ui/primitives/Panel';
import { CameraStage, type CameraStageStream } from '@/ui/telemetry/CameraStage';
import { MotorCard } from '@/ui/telemetry/MotorCard';
import { MotorDetailTable } from '@/ui/telemetry/MotorDetailTable';
import { LocationCard } from '@/ui/telemetry/LocationCard';
import { Metric } from '@/ui/data/Metric';
import { MetricGrid } from '@/ui/data/MetricGrid';
import { ActiveAlertsPanel } from '@/ui/telemetry/ActiveAlertsPanel';
import { ApplyImageDialog } from '@/ui/forms/ApplyImageDialog';
import { useMe } from '@/state/mode';
import { usePlatform } from '@/state/platform';
import { acknowledgeAlert, resolveAlert } from '@/state/useAlerts';
import { useRoverContext } from '../RoverContext';
import styles from './OverviewPanel.module.css';


// Render every camera the rover advertises, in the order it lists them (the
// agent sorts discovered devices). No hardcoded slot names — a camera shows up
// the moment it appears in waypoint.<id>.infra.camera.list.
function buildCameraStreams(whepBase: string, cameraNames: string[]): CameraStageStream[] {
  return cameraNames.map((name) => ({ name, whepUrl: `${whepBase}/camera/${name}/whep` }));
}

// The Overview reads the generic telemetry.uplink rail (the agent mirrors it from
// a connectivity module). It never knows about a specific module.
export function signalMetric(uplink: { online?: boolean; signalBars?: number; signalBarsMax?: number } | null) {
  if (!uplink) return { value: null, naReason: 'no connectivity module' };
  if (!uplink.online) return { value: null, naReason: 'uplink offline' };
  if (uplink.signalBars === undefined) return { value: null, naReason: 'ethernet uplink' };
  const max = uplink.signalBarsMax ?? 4;
  return { value: uplink.signalBars, unit: `/${max}` };
}

export function OverviewPanel() {
  const ctx = useRoverContext();
  const me = useMe();
  const platform = usePlatform(ctx.id);
  const [applyOpen, setApplyOpen] = useState(false);

  const cameraStreams = buildCameraStreams(ctx.whepBase, ctx.cameraNames);

  // Whole-rover motor view: render every servo id that has telemetry,
  // numerically ascending. No platform-level knowledge of slot roles —
  // arm/drill modules will show up here automatically the moment their
  // telemetry starts flowing on waypoint.<id>.telemetry.motors.
  const motorIds = Object.keys(ctx.motors)
    .map(Number)
    .filter((n) => Number.isFinite(n))
    .sort((a, b) => a - b);

  return (
    <div className={styles.layout} data-testid="panel-overview">
      <div className={styles.center}>
        {/* Position is null until a GPS module publishes telemetry.gps — the
            card then shows the rover; for now it defaults to Paris. */}
        <LocationCard roverId={ctx.id} position={null} />

        {cameraStreams.length > 0 ? (
          <CameraStage streams={cameraStreams} />
        ) : (
          <Panel title="CAMERAS" note="awaiting rover">
            <div className={styles.camPlaceholder}>N/A — no cameras advertised on this rover</div>
          </Panel>
        )}

        {platform.hasDrive && (
          <Panel title="DRIVE" note="live · 50 hz">
            <MetricGrid>
              <Metric label="Body vel" value={ctx.drive?.bodyVxMps ?? null}    unit="m/s"   naReason="no telemetry" tone="key" />
              <Metric label="Yaw rate" value={ctx.drive?.yawRateRadps ?? null} unit="rad/s" naReason="no telemetry" />
            </MetricGrid>
          </Panel>
        )}

        <Panel title="MOTORS" note={`${motorIds.length} active · STS3215`}>
          {motorIds.length === 0 ? (
            <div className={styles.motorsEmpty}>N/A — no motor telemetry on the bus</div>
          ) : (
            <div className={styles.motorsGrid}>
              {motorIds.map((motorId) => {
                const m = ctx.motors[motorId];
                return (
                  <MotorCard
                    key={motorId}
                    id={motorId}
                    currentA={m?.currentA ?? null}
                    temperatureC={m?.temperatureC ?? null}
                    actualVelRadps={m?.velocityRadps ?? null}
                  />
                );
              })}
            </div>
          )}
        </Panel>

        {motorIds.length > 0 && (
          <Panel title="MOTOR DETAIL" note="raw telemetry">
            <MotorDetailTable
              motors={ctx.motors}
              ids={motorIds}
            />
          </Panel>
        )}
      </div>

      <aside className={styles.rail}>
        <ActiveAlertsPanel
          alerts={ctx.roverAlerts}
          canControl={ctx.canControl}
          hideRover
          onAcknowledge={acknowledgeAlert}
          onResolve={resolveAlert}
        />
        <Panel title="POWER">
          <MetricGrid>
            <Metric label="Bus V"   value={ctx.power?.busVoltageV ?? null}             unit="V" naReason="no telemetry" tone="key" />
            <Metric label="Battery" value={ctx.power?.batteryPercent ?? null}                    naReason="no fuel-gauge module" />
            <Metric label="Draw"    value={ctx.power?.currentDrawA ?? null}             unit="A" naReason="no telemetry" />
            <Metric label="Runtime" value={ctx.power?.runtimeEstimateMinutes ?? null}            naReason="needs battery %" />
          </MetricGrid>
        </Panel>
        <Panel title="ORIENTATION">
          <MetricGrid>
            <Metric label="Heading"  value={null} naReason="no IMU" />
            <Metric label="Pitch"    value={null} naReason="no IMU" />
            <Metric label="Roll"     value={null} naReason="no IMU" />
            <Metric label="Position" value={null} naReason="no GPS" />
          </MetricGrid>
        </Panel>
        <Panel title="SYSTEM">
          <MetricGrid>
            <Metric label="CPU"  value={ctx.sys?.cpuPercent ?? null}   unit="%"  naReason="no telemetry" />
            <Metric label="Temp" value={ctx.sys?.temperatureC ?? null} unit="°C" naReason="no telemetry" />
            <Metric label="RTT"  value={ctx.sys?.linkRttMs ?? null}    unit="ms" naReason="no telemetry" />
            <Metric label="Signal" {...signalMetric(ctx.uplink)} />
          </MetricGrid>
          {me?.isAdmin && (
            <div className={styles.systemActions}>
              <Button onClick={() => setApplyOpen(true)}>Apply update…</Button>
            </div>
          )}
        </Panel>
      </aside>

      <ApplyImageDialog roverId={ctx.id} open={applyOpen} onClose={() => setApplyOpen(false)} />
    </div>
  );
}
