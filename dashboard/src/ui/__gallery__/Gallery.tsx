// dashboard/src/ui/__gallery__/Gallery.tsx
//
// Renders every component in every state. Reach this at /ui-gallery during dev.
// Update this file whenever a primitive or pattern changes — it's the canonical
// visual reference for spawned agents.
import React, { useState } from 'react';
import { Plus, SlidersHorizontal, AlertTriangle, Signal, Battery } from 'lucide-react';
import { Button } from '../primitives/Button';
import { Panel }  from '../primitives/Panel';
import { Chip }   from '../primitives/Chip';
import { Avatar } from '../primitives/Avatar';
import { BracketCorners } from '../primitives/BracketCorners';
import { Stack }  from '../primitives/Stack';
import { Metric } from '../data/Metric';
import { MetricGrid } from '../data/MetricGrid';
import { StatusDot }  from '../data/StatusDot';
import { LinearGauge } from '../data/LinearGauge';
import { SignalBars }  from '../data/SignalBars';
import { AxisBar } from '../data/AxisBar';
import { StatusPills } from '../telemetry/StatusPills';
import { EventLog } from '../telemetry/EventLog';
import { ConnectionPill } from '../telemetry/ConnectionPill';
import { ModeIndicator }  from '../telemetry/ModeIndicator';
import { MotorCard }      from '../telemetry/MotorCard';
import { JoystickPad }    from '../telemetry/JoystickPad';
import { CameraViewGalleryExample } from '../telemetry/CameraView.gallery';
import { RecordControl } from '../telemetry/RecordControl';
import { OperatorClock }  from '../nav/OperatorClock';
import { TabStrip } from '../nav/TabStrip';
import { MemoryRouter, Routes, Route } from 'react-router-dom';
import styles from './Gallery.module.css';

export function Gallery() {
  const [joy, setJoy] = useState({ vx: 0, yaw: 0 });
  return (
    <main className={styles.gallery}>
      <Section title="Buttons">
        <Stack>
          <Button>Default</Button>
          <Button variant="primary" icon={<Plus size={12} />}>Add rover</Button>
          <Button variant="danger">Emergency stop</Button>
          <Button disabled>Disabled</Button>
          <Button icon={<SlidersHorizontal size={12} />}>Filter</Button>
        </Stack>
      </Section>

      <Section title="Chips">
        <Stack>
          <Chip dot>Online</Chip>
          <Chip tone="caution" icon={<AlertTriangle size={12} />} number={2}>alerts</Chip>
          <Chip tone="fault">Offline</Chip>
          <Chip icon={<Battery size={12} />}>12.4 V</Chip>
          <Chip icon={<Signal  size={12} />}>5G · -68 dBm</Chip>
        </Stack>
      </Section>

      <Section title="Avatars + Clock">
        <Stack align="center">
          <Avatar initials="BE" />
          <Avatar initials="ab" />
          <OperatorClock />
        </Stack>
      </Section>

      <Section title="TabStrip · default">
        <MemoryRouter initialEntries={['/rover/demo/overview']}>
          <Routes>
            <Route path="/rover/:id/:tab" element={
              <TabStrip tabs={[
                { id: 'overview',   label: 'OVERVIEW',   to: '/rover/demo/overview' },
                { id: 'control',    label: 'CONTROL',    to: '/rover/demo/control' },
                { id: 'connection', label: 'CONNECTION', to: '/rover/demo/connection' },
                { id: 'logs',       label: 'LOGS',       to: '/rover/demo/logs' },
              ]} />
            } />
          </Routes>
        </MemoryRouter>
      </Section>

      <Section title="TabStrip · role-hidden (admin tab omitted for monitor)">
        <MemoryRouter initialEntries={['/rover/demo/overview']}>
          <Routes>
            <Route path="/rover/:id/:tab" element={
              <TabStrip tabs={[
                { id: 'overview', label: 'OVERVIEW', to: '/rover/demo/overview' },
                { id: 'control',  label: 'CONTROL',  to: '/rover/demo/control' },
              ]} />
            } />
          </Routes>
        </MemoryRouter>
      </Section>

      <Section title="TabStrip · overflow">
        <div style={{ maxWidth: 480 }}>
          <MemoryRouter initialEntries={['/rover/demo/overview']}>
            <Routes>
              <Route path="/rover/:id/:tab" element={
                <TabStrip tabs={[
                  { id: 'overview',   label: 'OVERVIEW',   to: '/rover/demo/overview' },
                  { id: 'control',    label: 'CONTROL',    to: '/rover/demo/control' },
                  { id: 'connection', label: 'CONNECTION', to: '/rover/demo/connection' },
                  { id: 'logs',       label: 'LOGS',       to: '/rover/demo/logs' },
                  { id: 'm-arm',      label: 'ARM',        to: '/rover/demo/m-arm' },
                  { id: 'm-drill',    label: 'DRILL',      to: '/rover/demo/m-drill' },
                  { id: 'm-cal',      label: 'CALIBRATION', to: '/rover/demo/m-cal' },
                  { id: 'm-mission',  label: 'MISSION',    to: '/rover/demo/m-mission' },
                ]} />
              } />
            </Routes>
          </MemoryRouter>
        </div>
      </Section>

      <Section title="Status dots">
        <Stack><StatusDot status="ok" label="Online" /><StatusDot status="warn" label="Safe" /><StatusDot status="fault" label="Fault" /><StatusDot status="off" label="Offline" /></Stack>
      </Section>

      <Section title="SignalBars">
        <Stack align="center">
          <SignalBars value={null} />
          <SignalBars value={0} showLabel />
          <SignalBars value={1} showLabel />
          <SignalBars value={2} showLabel />
          <SignalBars value={3} showLabel />
          <SignalBars value={4} showLabel />
        </Stack>
      </Section>

      <Section title="LinearGauge">
        <div style={{ width: 180 }}><LinearGauge value={0.87} /></div>
        <div style={{ width: 180 }}><LinearGauge value={0.22} /></div>
        <div style={{ width: 180 }}><LinearGauge value={0.05} /></div>
      </Section>

      <Section title="AxisBar — cmd vs meas, saturation, no telemetry">
        <div style={{ width: 180 }}>
          <AxisBar cmd={0.4} meas={0.36} max={0.6} aria-label="tracking" />
          <AxisBar cmd={-0.5} meas={-0.59} max={0.6} aria-label="saturated" />
          <AxisBar cmd={0.25} meas={null} max={0.6} aria-label="no telemetry" />
        </div>
      </Section>

      <Section title="StatusPills">
        <div style={{ width: 220 }}>
          <StatusPills pills={[
            { label: 'core', tone: 'ok' },
            { label: 'agent', tone: 'ok' },
            { label: 'cam', tone: 'warn' },
            { label: 'link', tone: 'off' },
          ]} />
        </div>
      </Section>

      <Section title="EventLog">
        <div style={{ width: 260 }}>
          <EventLog events={[
            { atMs: 1718200951000, text: 'mode → manual' },
            { atMs: 1718200940000, text: 'alert raised (1 active)', tone: 'warn' },
            { atMs: 1718200931000, text: 'mode → estop', tone: 'fault' },
          ]} />
        </div>
      </Section>

      <Section title="Metrics — including N/A states">
        <MetricGrid>
          <Metric label="Bus voltage"  value={12.4} unit="V" tone="key" />
          <Metric label="Body velocity" value={0.42} unit="m/s" tone="key" />
          <Metric label="Yaw rate"     value={-0.18} unit="rad/s" />
          <Metric label="Battery"      value={null} naReason="no fuel-gauge module" />
          <Metric label="Heading"      value={null} naReason="no IMU" />
          <Metric label="Cell signal"  value={null} naReason="UMR API tba" />
        </MetricGrid>
      </Section>

      <Section title="Connection + Mode">
        <Stack>
          <ConnectionPill conn={{ kind: 'direct', rttMs: 4 }} />
          <ConnectionPill conn={{ kind: 'proxy',  rttMs: 32 }} />
          <ConnectionPill conn={{ kind: 'offline' }} />
          <ModeIndicator mode="manual" />
          <ModeIndicator mode="safe" />
          <ModeIndicator mode="autonomous" />
          <ModeIndicator mode="estop" />
          <ModeIndicator mode="unknown" />
        </Stack>
      </Section>

      <Section title="Motors (drive servos)">
        <Stack>
          <MotorCard id={7}  position="BL" currentA={2.0} temperatureC={42} />
          <MotorCard id={8}  position="BR" currentA={2.3} temperatureC={44} />
          <MotorCard id={9}  position="FR" currentA={2.4} temperatureC={58} />
          <MotorCard id={10} position="FL" currentA={2.1} temperatureC={43} />
        </Stack>
      </Section>

      <Section title="Joystick (drag the knob)">
        <Stack direction="column" gap={12}>
          <JoystickPad onChange={setJoy} />
          <code style={{ fontFamily: 'var(--font-mono)', fontSize: 12, color: 'var(--color-fg-2)' }}>
            vx={joy.vx.toFixed(2)} &nbsp; yaw={joy.yaw.toFixed(2)}
          </code>
        </Stack>
      </Section>

      <Section title="Panel + Bracket corners">
        <div style={{ position: 'relative', display: 'inline-block' }}>
          <Panel title="DRIVE" note="live · 50 hz">
            <MetricGrid>
              <Metric label="Body vel" value={0.42} unit="m/s" tone="key" />
              <Metric label="Yaw"      value={-0.18} unit="rad/s" />
            </MetricGrid>
          </Panel>
          <BracketCorners full />
        </div>
      </Section>

      <Section title="Record control (idle · blocked · recording)">
        <Stack direction="row" gap={16}>
          <RecordControl roverId="rover-dev" recorder={{ state: 'idle', episodeId: '', elapsedS: 0, bytes: 0, canStart: true, reason: '' }} />
          <RecordControl roverId="rover-dev" recorder={{ state: 'idle', episodeId: '', elapsedS: 0, bytes: 0, canStart: false, reason: 'low disk: 120 MiB free' }} />
          <RecordControl roverId="rover-dev" recorder={{ state: 'recording', episodeId: 'ep-1', elapsedS: 98, bytes: 73400320, canStart: false, reason: '' }} />
        </Stack>
      </Section>

      <Section title="Camera view (synthetic WHEP)">
        <CameraViewGalleryExample />
      </Section>
    </main>
  );
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className={styles.section}>
      <h2 className={styles.heading}>{title}</h2>
      <div className={styles.row}>{children}</div>
    </section>
  );
}
