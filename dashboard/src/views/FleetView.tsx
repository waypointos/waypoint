// dashboard/src/views/FleetView.tsx
import { useEffect, useState } from 'react';
import { Plus, SlidersHorizontal } from 'lucide-react';
import { Sidebar, SidebarBrand } from '@/ui/nav/Sidebar';
import { TopBar } from '@/ui/nav/TopBar';
import { OperatorClock } from '@/ui/nav/OperatorClock';
import { AlertCountBadge } from '@/ui/nav/AlertCountBadge';
import { Button } from '@/ui/primitives/Button';
import { Panel } from '@/ui/primitives/Panel';
import { useAlerts } from '@/state/useAlerts';
import { RoverCard, RoverSummary } from './RoverCard';
import { useTelemetry } from '@/state/useTelemetry';
import { pbFromBinary } from '@/state/protobuf';
import { SystemTelemetry } from '../../../protocol/gen/ts/messages/system_pb';
import { PowerTelemetry } from '../../../protocol/gen/ts/messages/power_pb';
import { fetchMe, Me } from '@/state/mode';
import { useFleet, isOnline } from '@/state/useFleet';
import { EnrollRoverWizard } from '@/ui/forms/EnrollRoverWizard';
import styles from './FleetView.module.css';

// ── Local mode (single rover, Phase 1 behaviour) ──────────────────────────────

function LocalFleetView({ roverId }: { roverId: string }) {
  const sys = useTelemetry(`waypoint.${roverId}.telemetry.system`, (b) =>
    pbFromBinary<SystemTelemetry>(SystemTelemetry, b),
  );
  const pwr = useTelemetry(`waypoint.${roverId}.telemetry.power`, (b) =>
    pbFromBinary<PowerTelemetry>(PowerTelemetry, b),
  );
  const { counts: alertCounts } = useAlerts();

  const rover: RoverSummary = {
    id:         roverId,
    nick:       'Devbox · Local',
    status:     sys ? 'online' : 'offline',
    mode:       'Manual',
    busVoltage: pwr?.busVoltageV ?? null,
    link:       sys ? `Direct · ${(sys.linkRttMs ?? 0).toFixed(0)} ms` : null,
    version:    sys?.imageVersion ?? null,
  };

  return (
    <div className={styles.shell}>
      <SidebarBrand />
      <TopBar
        left={
          <div className={styles.title}>
            <h1>// FLEET</h1>
            <div className={styles.sub}>{rover.status === 'online' ? '1 ROVER · ONLINE' : '1 ROVER · OFFLINE'}</div>
          </div>
        }
        right={
          <>
            <OperatorClock />
            <AlertCountBadge counts={alertCounts} />
          </>
        }
      />
      <Sidebar
        meta={
          <>
            <span>Local</span>
            <span style={{ fontFamily: 'var(--font-mono)' }}>{sys?.imageVersion ?? 'v?'}</span>
          </>
        }
      />
      <main className={styles.body}>
        <section className={styles.content}>
          <div className={styles.toolbar}>
            <div className={styles.left}>
              <Button icon={<SlidersHorizontal size={12} />}>Filter</Button>
              <Button>Sort: Last seen</Button>
            </div>
          </div>
          <div className={styles.grid}>
            <RoverCard rover={rover} />
          </div>
        </section>
      </main>
    </div>
  );
}

// ── Proxy mode (fleet from /api/rovers) ───────────────────────────────────────

function ProxyFleetView() {
  const { rovers, loading, refresh } = useFleet();
  const { counts: alertCounts } = useAlerts();
  const [enrollOpen, setEnrollOpen] = useState(false);

  const onlineCount = rovers.filter((r) => isOnline(r.lastSeen)).length;
  const subLine = loading
    ? 'LOADING…'
    : rovers.length === 0
    ? 'NO ROVERS'
    : `${rovers.length} ROVER${rovers.length === 1 ? '' : 'S'} · ${onlineCount} ONLINE`;

  return (
    <div className={styles.shell}>
      <SidebarBrand />
      <TopBar
        left={
          <div className={styles.title}>
            <h1>// FLEET</h1>
            <div className={styles.sub}>{subLine}</div>
          </div>
        }
        right={
          <>
            <OperatorClock />
            <AlertCountBadge counts={alertCounts} />
          </>
        }
      />
      <Sidebar />
      <main className={styles.body}>
        <section className={styles.content}>
          <div className={styles.toolbar}>
            <div className={styles.left}>
              <Button icon={<SlidersHorizontal size={12} />}>Filter</Button>
              <Button>Sort: Last seen</Button>
            </div>
            <Button
              variant="primary"
              icon={<Plus size={12} />}
              onClick={() => setEnrollOpen(true)}
            >
              Add rover
            </Button>
          </div>

          {rovers.length === 0 && !loading ? (
            <Panel>
              <div className={styles.empty}>
                <div className={styles.emptyTitle}>No rovers yet</div>
                <div className={styles.emptyHint}>Click "Add rover" to enroll one.</div>
              </div>
            </Panel>
          ) : (
            <div className={styles.grid}>
              {rovers.map((r) => {
                const online = isOnline(r.lastSeen);
                const summary: RoverSummary = {
                  id:         r.id,
                  nick:       r.name,
                  status:     online ? 'online' : 'offline',
                  mode:       r.role,
                  busVoltage: null,
                  link:       r.lastSeen
                    ? online
                      ? 'Proxy · online'
                      : `Last seen ${new Date(r.lastSeen).toLocaleTimeString()}`
                    : null,
                  version:    r.imageVersion ?? null,
                };
                return <RoverCard key={r.id} rover={summary} />;
              })}
            </div>
          )}
        </section>
      </main>
      <EnrollRoverWizard
        open={enrollOpen}
        onClose={() => {
          setEnrollOpen(false);
          refresh();
        }}
        onEnrolled={refresh}
      />
    </div>
  );
}

// ── Top-level branch ──────────────────────────────────────────────────────────

export function FleetView() {
  const [me, setMe] = useState<Me | null>(null);

  useEffect(() => {
    fetchMe().then(setMe);
  }, []);

  if (!me) {
    // Render nothing while the single /api/me round-trip completes (fast on LAN).
    return null;
  }

  if (me.mode === 'local') {
    return <LocalFleetView roverId={me.roverId ?? 'rover-dev'} />;
  }

  return <ProxyFleetView />;
}
