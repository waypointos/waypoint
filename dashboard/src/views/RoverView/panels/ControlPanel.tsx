// Operator cockpit: cameras, joystick, mode switch, e-stop, motors row, and a
// launch into the full-screen Teleop console. Drive metrics are inline (small
// strip) — the dense telemetry rail lives on Overview.
import { useState } from 'react';
import { Link } from 'react-router-dom';
import { Chip } from '@/ui/primitives/Chip';
import { Panel } from '@/ui/primitives/Panel';
import { CameraStage, type CameraStageStream } from '@/ui/telemetry/CameraStage';
import { JoystickPad } from '@/ui/telemetry/JoystickPad';
import { MotorCard } from '@/ui/telemetry/MotorCard';
import { MotorDetailTable } from '@/ui/telemetry/MotorDetailTable';
import { ModeControl } from '@/ui/telemetry/ModeControl';
import { EstopButton } from '@/ui/telemetry/EstopButton';
import { SpeedPresets } from '@/ui/telemetry/SpeedPresets';
import { Metric } from '@/ui/data/Metric';
import { MetricGrid } from '@/ui/data/MetricGrid';
import { useDriveCmd } from '@/state/useDriveCmd';
import { useDriveInput } from '@/state/useDriveInput';
import { useModeCommand } from '@/state/useModeCommand';
import { applySpeedScale, type SpeedPreset } from '@/state/speed';
import { stickToPhysical, wheelTargets } from '@/state/kinematics';
import {
  leftWheelBusIds,
  rightWheelBusIds,
  usePlatform,
  wheelBusIds,
} from '@/state/platform';
import { useRoverContext } from '../RoverContext';
import styles from './ControlPanel.module.css';

const DRIVE_POSITIONS: Record<number, string> = { 7: 'BL', 8: 'BR', 9: 'FR', 10: 'FL' };

// Render every camera the rover advertises, in the order it lists them (the
// agent sorts discovered devices). No hardcoded slot names — a camera shows up
// the moment it appears in waypoint.<id>.infra.camera.list.
function buildCameraStreams(whepBase: string, cameraNames: string[]): CameraStageStream[] {
  return cameraNames.map((name) => ({ name, whepUrl: `${whepBase}/camera/${name}/whep` }));
}

export function ControlPanel({ active = true }: { roverId?: string; active?: boolean } = {}) {
  const ctx = useRoverContext();
  const platform = usePlatform(ctx.id);
  const cameraStreams = buildCameraStreams(ctx.whepBase, ctx.cameraNames);

  // Drive commands require: connected + control role + core in MANUAL mode.
  // Any other combination disables the stick — including the bootstrap case
  // where we have not yet received an event.mode (mode === 'unknown').
  const offline = ctx.connection.kind === 'offline';
  const inManual = ctx.mode === 'manual';
  const driveDisabled = !ctx.canControl || offline || !inManual;
  const driveDisabledReason = !ctx.canControl
    ? 'monitor role'
    : offline
      ? 'offline · awaiting rover'
      : ctx.mode === 'unknown'
        ? 'mode unknown · awaiting core'
        : ctx.mode === 'safe'
          ? 'in safe mode'
          : ctx.mode === 'autonomous'
            ? 'auto mode active'
            : ctx.mode === 'estop'
              ? 'e-stop engaged'
              : undefined;

  // Input source (virtual joystick or gamepad) + speed shaping live here so the
  // motor cards display the same scaled target we publish.
  const { source, setSource, gamepadConnected, setVirtual, stick } = useDriveInput();
  const [preset, setPreset] = useState<SpeedPreset>('cruise');
  const [cap, setCap] = useState(1);
  const { pending, requestMode, requestEstop, requestRecover } = useModeCommand(ctx.id, ctx.mode);

  const scaled = applySpeedScale(stick, preset, cap);
  const effectiveStick = driveDisabled ? { vx: 0, yaw: 0 } : scaled;

  // Publish at 50 Hz when the operator has control authority AND this panel
  // is the active tab. Monitor accounts and hidden mounts both stay paused so
  // they never interleave zero frames with another tab's real commands. The
  // hook also gates on the confirmed mode: only Manual publishes cmd.drive.
  useDriveCmd(ctx.id, effectiveStick, { paused: !ctx.canControl || !active, mode: ctx.mode });

  // Compute per-wheel target rad/s using the same skid-steer formula the core
  // applies. Display only — the rover does its own conversion from DriveCommand.
  // Geometry and wheel grouping come from the runtime platform, so a drive-less
  // platform yields no targets and the drive section below is hidden.
  const physical = stickToPhysical(effectiveStick);
  const driveIds = wheelBusIds(platform);
  const targetByMotor: Record<number, number> = {};
  if (platform.kinematics) {
    const targets = wheelTargets(
      { wheelRadiusM: platform.kinematics.wheelRadiusM, trackWidthM: platform.kinematics.trackWidthM },
      physical.vxMps,
      physical.yawRadps,
    );
    for (const id of leftWheelBusIds(platform)) targetByMotor[id] = targets.left;
    for (const id of rightWheelBusIds(platform)) targetByMotor[id] = targets.right;
  }

  return (
    <div className={styles.layout} data-testid="panel-control">
      <div className={styles.left}>
        {cameraStreams.length > 0 ? (
          <CameraStage streams={cameraStreams} />
        ) : (
          <Panel title="CAMERAS" note="awaiting rover">
            <div className={styles.camPlaceholder}>N/A — no cameras advertised on this rover</div>
          </Panel>
        )}

        {platform.hasDrive && (
          <>
            <Panel title="DRIVE" note="live · 50 hz">
              <MetricGrid>
                <Metric label="Cmd vx"  value={physical.vxMps}                  unit="m/s"   tone="key" />
                <Metric label="Cmd yaw" value={physical.yawRadps}               unit="rad/s" />
                <Metric label="Body vel" value={ctx.drive?.bodyVxMps ?? null}    unit="m/s"   naReason="no telemetry" />
                <Metric label="Yaw rate" value={ctx.drive?.yawRateRadps ?? null} unit="rad/s" naReason="no telemetry" />
              </MetricGrid>
            </Panel>

            <Panel title="MOTORS" note={`${driveIds.length} drive · STS3215`}>
              <div className={styles.motorsGrid}>
                {driveIds.map((motorId) => {
                  const m = ctx.motors[motorId];
                  return (
                    <MotorCard
                      key={motorId}
                      id={motorId}
                      position={DRIVE_POSITIONS[motorId] ?? `?${motorId}`}
                      currentA={m?.currentA ?? null}
                      temperatureC={m?.temperatureC ?? null}
                      targetVelRadps={targetByMotor[motorId]}
                      actualVelRadps={m?.velocityRadps ?? null}
                    />
                  );
                })}
              </div>
            </Panel>

            <Panel title="MOTOR DETAIL" note="raw telemetry">
              <MotorDetailTable
                motors={ctx.motors}
                ids={driveIds}
                positions={DRIVE_POSITIONS}
                targets={targetByMotor}
              />
            </Panel>
          </>
        )}
      </div>

      <aside className={styles.right}>
        {platform.hasDrive && (
          <Link to={`/rover/${ctx.id}/teleop`} className={styles.teleopLink}>⤢ Teleop console</Link>
        )}

        <div className={styles.ctrl}>
          <h3>Control</h3>
          {platform.hasDrive && !ctx.canControl ? (
            <div className={styles.readOnly}>
              <Chip tone="caution">READ-ONLY</Chip>
              <div className={styles.readOnlyHint}>Monitor role — no drive commands</div>
            </div>
          ) : platform.hasDrive ? (
            <>
              <JoystickPad
                onChange={setVirtual}
                disabled={driveDisabled}
                disabledReason={driveDisabledReason}
              />
              <div className={styles.srcSwitch}>
                <button
                  type="button"
                  className={source === 'virtual' ? styles.srcOn : undefined}
                  onClick={() => setSource('virtual')}
                >Virtual</button>
                <button
                  type="button"
                  className={source === 'gamepad' ? styles.srcOn : undefined}
                  disabled={!gamepadConnected}
                  onClick={() => setSource('gamepad')}
                >Gamepad{gamepadConnected ? '' : ' —'}</button>
              </div>
              <div className={styles.readouts}>
                <div><div className={styles.l}>Throttle</div><div className={styles.v}>{(effectiveStick.vx * 100).toFixed(0)}%</div></div>
                <div><div className={styles.l}>Steer</div>   <div className={styles.v}>{(effectiveStick.yaw * 100).toFixed(0)}%</div></div>
              </div>
              <SpeedPresets preset={preset} cap={cap} onPreset={setPreset} onCap={setCap} disabled={driveDisabled} />
            </>
          ) : null}

          <div className={styles.modeBlock}>
            <ModeControl
              mode={ctx.mode}
              pending={pending}
              disabled={!ctx.canControl || offline}
              onSelect={requestMode}
            />
            <div className={styles.modeNote}>
              {ctx.mode === 'unknown'
                ? 'Mode: unknown — no event from core'
                : ctx.mode === 'estop'
                  ? 'Mode: E-STOP engaged'
                  : pending
                    ? `Switching to ${pending}…`
                    : `Mode: ${ctx.mode}`}
            </div>
            {ctx.canControl && (
              <EstopButton
                mode={ctx.mode}
                offline={offline}
                clearing={pending === 'clearing'}
                onEngage={requestEstop}
                onClear={requestRecover}
              />
            )}
          </div>
        </div>

        <Panel title="ORIENTATION">
          <MetricGrid>
            <Metric label="Heading"  value={null} naReason="no IMU" />
            <Metric label="Pitch"    value={null} naReason="no IMU" />
            <Metric label="Roll"     value={null} naReason="no IMU" />
            <Metric label="Position" value={null} naReason="no GPS" />
          </MetricGrid>
        </Panel>
      </aside>
    </div>
  );
}
