// Top-level Map view: a full-bleed 3D map with a floating, blurred rover list
// overlaid. Rovers with no GPS fix are shown at Paris (selecting one flies the
// camera). Sidebar is collapsed to icons to maximise the map.
import { useEffect, useState } from 'react';
import { Sidebar, SidebarBrand } from '@/ui/nav/Sidebar';
import { TopBar } from '@/ui/nav/TopBar';
import { OperatorClock } from '@/ui/nav/OperatorClock';
import { AlertCountBadge } from '@/ui/nav/AlertCountBadge';
import { LocationMap, type RoverMarker } from '@/ui/telemetry/LocationMap';
import { useAlerts } from '@/state/useAlerts';
import { useFleet, isOnline } from '@/state/useFleet';
import { fetchMe, Me } from '@/state/mode';
import styles from './MapView.module.css';

type Row = { id: string; nick: string; online: boolean };

export function MapView() {
  const [me, setMe] = useState<Me | null>(null);
  const [focusId, setFocusId] = useState<string | null>(null);
  const fleet = useFleet(); // returns [] in local mode (no /api/rovers)
  const { counts } = useAlerts();

  useEffect(() => {
    fetchMe().then(setMe);
  }, []);

  if (!me) return null;

  const localMode = me.mode === 'local';
  const rovers: Row[] = localMode
    ? [{ id: me.roverId ?? 'rover-dev', nick: 'Devbox · Local', online: true }]
    : fleet.rovers.map((r) => ({ id: r.id, nick: r.name, online: isOnline(r.lastSeen) }));

  // No GPS data anywhere yet → every rover defaults to Paris.
  const markers: RoverMarker[] = rovers.map((r) => ({ id: r.id, nick: r.nick, position: null }));
  const onlineCount = rovers.filter((r) => r.online).length;
  const subLine = rovers.length === 0
    ? 'NO ROVERS'
    : `${rovers.length} ROVER${rovers.length === 1 ? '' : 'S'} · ${onlineCount} ONLINE`;

  return (
    <div className={styles.shell}>
      <SidebarBrand collapsed />
      <TopBar
        left={
          <div className={styles.title}>
            <h1>// MAP</h1>
            <div className={styles.sub}>{subLine}</div>
          </div>
        }
        right={
          <>
            <OperatorClock />
            <AlertCountBadge counts={counts} />
          </>
        }
      />
      <Sidebar collapsed />
      <main className={styles.body}>
        <div className={styles.stage}>
          <LocationMap pitched markers={markers} focusId={focusId} className={styles.map} />

          <aside className={styles.floatCard}>
            <div className={styles.cardTitle}>
              <span><span className={styles.bracket}>[</span>ROVERS<span className={styles.bracket}>]</span></span>
              <span className={styles.count}>{rovers.length}</span>
            </div>
            <div className={styles.list}>
              {rovers.length === 0 ? (
                <div className={styles.empty}>No rovers</div>
              ) : (
                rovers.map((r) => (
                  <button
                    key={r.id}
                    type="button"
                    className={[styles.row, focusId === r.id && styles.rowActive].filter(Boolean).join(' ')}
                    onClick={() => setFocusId(r.id)}
                  >
                    <span className={[styles.dot, r.online ? styles.online : styles.offline].join(' ')} />
                    <span className={styles.rowMeta}>
                      <span className={styles.nick}>{r.nick}</span>
                      <span className={styles.loc}>Paris · no fix</span>
                    </span>
                  </button>
                ))
              )}
            </div>
          </aside>
        </div>
      </main>
    </div>
  );
}
