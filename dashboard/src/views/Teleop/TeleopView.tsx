// dashboard/src/views/Teleop/TeleopView.tsx
//
// Full-screen FPV driving console. Reuses the rover subscriptions via
// useRoverData, then overlays the mission HUD on a full-bleed camera:
// top strip (mode / link / exit), telemetry rail + subsystem pills, session
// event log, joystick, drive cluster with cmd-vs-meas bars, E-stop.
// Esc / Exit returns to the Control tab.
import { useEffect, useRef, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { CameraStage, type CameraStageStream } from '@/ui/telemetry/CameraStage';
import { JoystickPad } from '@/ui/telemetry/JoystickPad';
import { Reticle } from '@/ui/telemetry/Reticle';
import { HeadingTape } from '@/ui/telemetry/HeadingTape';
import { SpeedReadout } from '@/ui/telemetry/SpeedReadout';
import { ModeControl } from '@/ui/telemetry/ModeControl';
import { ModeIndicator } from '@/ui/telemetry/ModeIndicator';
import { SpeedPresets } from '@/ui/telemetry/SpeedPresets';
import { EstopButton } from '@/ui/telemetry/EstopButton';
import { RecordControl } from '@/ui/telemetry/RecordControl';
import { StatusPills, type Pill } from '@/ui/telemetry/StatusPills';
import { EventLog } from '@/ui/telemetry/EventLog';
import { AxisBar } from '@/ui/data/AxisBar';
import { Metric } from '@/ui/data/Metric';
import { signedFixed } from '@/ui/format';
import { useDriveCmd } from '@/state/useDriveCmd';
import { useDriveInput } from '@/state/useDriveInput';
import { useModeCommand } from '@/state/useModeCommand';
import { applySpeedScale, type SpeedPreset } from '@/state/speed';
import { stickToPhysical, wheelTargets, MAX_VX_MPS, MAX_YAW_RADPS } from '@/state/kinematics';
import { usePlatform } from '@/state/platform';
import { useRoverData } from '@/views/RoverView/useRoverData';
import { useGamepadPublish } from '@/state/useGamepadPublish';
import { useMe } from '@/state/mode';
import { TeleopWindowHost } from './TeleopWindowHost';
import { resolveTeleopWindows } from './teleopWindows';
import { useSessionEvents } from './useSessionEvents';
import { useFreshness } from './useFreshness';
import styles from './TeleopView.module.css';

const WHEEL_POSITIONS = [
  { key: 'FL', position: 'front_left' },
  { key: 'FR', position: 'front_right' },
  { key: 'BL', position: 'back_left' },
  { key: 'BR', position: 'back_right' },
] as const;

export function TeleopView() {
  const { id = 'rover-dev' } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const ctx = useRoverData(id);
  const platform = usePlatform(ctx.id);

  const offline = ctx.connection.kind === 'offline';
  const driveDisabled = !ctx.canControl || offline || ctx.mode !== 'manual';
  const driveDisabledReason = !ctx.canControl ? 'monitor role'
    : offline ? 'offline · awaiting rover'
    : ctx.mode === 'estop' ? 'e-stop engaged'
    : ctx.mode === 'safe' ? 'in safe mode'
    : ctx.mode === 'autonomous' ? 'auto mode active'
    : ctx.mode === 'unknown' ? 'mode unknown · awaiting core'
    : undefined;

  const { setVirtual, stick } = useDriveInput();
  const [preset, setPreset] = useState<SpeedPreset>('cruise');
  const [cap, setCap] = useState(1);
  const { pending, requestMode, requestEstop, requestRecover } = useModeCommand(ctx.id, ctx.mode);

  const scaled = applySpeedScale(stick, preset, cap);
  const effectiveStick = driveDisabled ? { vx: 0, yaw: 0 } : scaled;
  useDriveCmd(ctx.id, effectiveStick, { paused: !ctx.canControl, mode: ctx.mode });

  useGamepadPublish(ctx.id);
  const me = useMe();
  const teleopWindows = resolveTeleopWindows(ctx.modules ?? [], me?.mode ?? 'local');

  // Esc exits back to the Control tab.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') navigate(`/rover/${id}/control`); };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [id, navigate]);

  // Session event log: mode transitions, link changes, alert raises.
  const { events, log } = useSessionEvents();
  const prevMode = useRef<string | null>(null);
  useEffect(() => {
    if (prevMode.current !== null && prevMode.current !== ctx.mode) {
      log(`mode → ${ctx.mode}`, ctx.mode === 'estop' ? 'fault' : undefined);
    }
    prevMode.current = ctx.mode;
  }, [ctx.mode, log]);
  const prevLink = useRef<string | null>(null);
  useEffect(() => {
    if (prevLink.current !== null && prevLink.current !== ctx.connection.kind) {
      log(`link → ${ctx.connection.kind}`, ctx.connection.kind === 'offline' ? 'warn' : undefined);
    }
    prevLink.current = ctx.connection.kind;
  }, [ctx.connection.kind, log]);
  const prevAlerts = useRef(0);
  useEffect(() => {
    const n = ctx.roverAlerts.length;
    if (n > prevAlerts.current) log(`alert raised (${n} active)`, 'warn');
    prevAlerts.current = n;
  }, [ctx.roverAlerts.length, log]);
  useEffect(() => {
    if (!ctx.recorder) return;
    if (ctx.recorder.state === 'recording') log(`rec ● ${ctx.recorder.episodeId}`);
    else if (ctx.recorder.reason) log(`rec stopped: ${ctx.recorder.reason}`);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ctx.recorder?.state]);

  // Subsystem pills from telemetry freshness, not just last-known values.
  const driveAge = useFreshness(ctx.drive);
  const sysAge = useFreshness(ctx.sys);
  const pills: Pill[] = [
    { label: 'core',  tone: driveAge === null ? 'off' : driveAge < 2_000 ? 'ok' : 'warn' },
    { label: 'agent', tone: sysAge === null ? 'off' : sysAge < 5_000 ? 'ok' : 'warn' },
    { label: 'cam',   tone: ctx.cameraNames.length > 0 ? 'ok' : 'off' },
    { label: 'link',  tone: offline ? 'off' : 'ok' },
  ];

  // Drive cluster: commanded physical values + per-wheel targets vs measured.
  const physical = stickToPhysical(effectiveStick);
  const wheelByName = new Map(platform.joints.map((j) => [j.name, j.busId]));
  const kin = platform.kinematics;
  const wheelMax = kin
    ? (MAX_VX_MPS + (MAX_YAW_RADPS * kin.trackWidthM) / 2) / kin.wheelRadiusM
    : 1;
  const sideTargets = kin
    ? wheelTargets({ wheelRadiusM: kin.wheelRadiusM, trackWidthM: kin.trackWidthM }, physical.vxMps, physical.yawRadps)
    : null;
  const wheels = kin && sideTargets
    ? WHEEL_POSITIONS.flatMap(({ key, position }) => {
        const busId = wheelByName.get(kin.wheels[position] ?? '');
        if (busId === undefined) return [];
        const cmd = position.endsWith('left') ? sideTargets.left : sideTargets.right;
        return [{ key, busId, cmd, meas: ctx.motors[busId]?.velocityRadps ?? null }];
      })
    : [];

  const uplinkSignal = ctx.uplink?.signalBars !== undefined && ctx.uplink?.signalBarsMax !== undefined
    ? `${ctx.uplink.signalBars}/${ctx.uplink.signalBarsMax}`
    : ctx.uplink?.mode === 'ethernet' ? 'eth' : null;

  const streams: CameraStageStream[] = ctx.cameraNames.map((name) => ({
    name, whepUrl: `${ctx.whepBase}/camera/${name}/whep`,
  }));

  return (
    <div className={styles.console} data-testid="teleop">
      <div className={styles.stage}>
        {streams.length > 0
          ? <CameraStage streams={streams} floatingPip mainHudShown={false} />
          : <div className={styles.noCam}>N/A — no cameras advertised</div>}

        <Reticle drift={effectiveStick} />

        <header className={styles.strip}>
          <span className={styles.brand}>[ waypoint · {ctx.id} · teleop ]</span>
          {ctx.canControl
            ? <ModeControl mode={ctx.mode} pending={pending} disabled={offline} size="sm" onSelect={requestMode} />
            : <ModeIndicator mode={ctx.mode} />}
          {ctx.canControl && <RecordControl roverId={id} recorder={ctx.recorder} />}
          {/* Absolutely centred over the strip so it tracks the viewport
              midpoint regardless of the left/right group widths. */}
          <div className={styles.stripCenter}>
            <HeadingTape yawRateRadps={ctx.drive?.yawRateRadps} />
          </div>
          <span className={styles.stripSpacer} />
          <span className={styles.link} data-link>
            {ctx.connection.kind === 'offline'
              ? 'link offline'
              : `${ctx.connection.kind} · ${ctx.connection.rttMs} ms`}
          </span>
          <Link to={`/rover/${id}/control`} className={styles.exit}>⤢ Exit</Link>
        </header>

        {ctx.mode === 'estop' && (
          <div className={styles.banner} role="alert">
            [ e-stop engaged ]<small>drive inhibited · clear to resume</small>
          </div>
        )}

        <div className={styles.left}>
          <aside className={styles.rail}>
            <div className={styles.railTitle}>rover</div>
            <Metric label="Bat" value={ctx.power?.busVoltageV ?? null} unit="V"
              naReason="no power telemetry" format={(v) => v.toFixed(2)} />
            <Metric label="Draw" value={ctx.power?.currentDrawA ?? null} unit="A"
              naReason="no power telemetry" />
            <Metric label="Core temp" value={ctx.sys?.temperatureC ?? null} unit="°C"
              naReason="no system telemetry" format={(v) => v.toFixed(0)} />
            <Metric label="CPU" value={ctx.sys?.cpuPercent ?? null} unit="%"
              naReason="no system telemetry" format={(v) => v.toFixed(0)} />
            <Metric label="Signal" value={uplinkSignal} naReason="no uplink module" />
            <Metric label="Pitch / roll" value={null} naReason="no imu fitted" />
            <div className={styles.railPills}><StatusPills pills={pills} /></div>
          </aside>

          <div className={styles.events}><EventLog events={events} max={5} /></div>
        </div>

        <div className={styles.tr}>
          <SpeedReadout
            mps={ctx.drive?.bodyVxMps ?? null}
            cmdMps={physical.vxMps}
            yawRadps={ctx.drive?.yawRateRadps ?? null}
            preset={preset}
            cap={cap}
          />
        </div>

        {ctx.canControl && (
          <div className={styles.bl}>
            <JoystickPad onChange={setVirtual} disabled={driveDisabled} disabledReason={driveDisabledReason} />
            <div className={styles.stickVals}>
              <span>vx {signedFixed(effectiveStick.vx)}</span>
              <span>yaw {signedFixed(effectiveStick.yaw)}</span>
            </div>
          </div>
        )}

        <div className={styles.drive}>
          <div className={styles.dcol}>
            <div className={styles.dhd}>speed</div>
            <SpeedPresets preset={preset} cap={cap} onPreset={setPreset} onCap={setCap} disabled={driveDisabled} />
          </div>
          <div className={styles.dcol}>
            <div className={styles.dhd}>body · cmd ▮ / meas ▯</div>
            <div className={styles.axes}>
              <span className={styles.axk}>vx</span>
              <AxisBar cmd={physical.vxMps} meas={ctx.drive?.bodyVxMps ?? null} max={MAX_VX_MPS} aria-label="Body vx" />
              <span className={styles.axv}>{ctx.drive?.bodyVxMps == null ? 'N/A' : signedFixed(ctx.drive.bodyVxMps)}<small> m/s</small></span>
              <span className={styles.axk}>yaw</span>
              <AxisBar cmd={physical.yawRadps} meas={ctx.drive?.yawRateRadps ?? null} max={MAX_YAW_RADPS} aria-label="Body yaw" />
              <span className={styles.axv}>{ctx.drive?.yawRateRadps == null ? 'N/A' : signedFixed((ctx.drive.yawRateRadps * 180) / Math.PI, 1)}<small> °/s</small></span>
            </div>
          </div>
          {wheels.length > 0 && (
            <div className={styles.dcol}>
              <div className={styles.dhd}>wheels · rad/s</div>
              <div className={styles.wheels}>
                {wheels.map((w) => (
                  <div key={w.key} className={styles.wheel} data-wheel={w.key}>
                    <span className={styles.axk}>{w.key}</span>
                    <AxisBar cmd={w.cmd} meas={w.meas} max={wheelMax} aria-label={`Wheel ${w.key}`} />
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>

        {ctx.canControl && (
          <div className={styles.br}>
            <EstopButton
              mode={ctx.mode}
              offline={offline}
              clearing={pending === 'clearing'}
              onEngage={requestEstop}
              onClear={requestRecover}
            />
          </div>
        )}

        <TeleopWindowHost roverId={ctx.id} windows={teleopWindows} />
      </div>
    </div>
  );
}
