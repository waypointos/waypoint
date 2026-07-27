// dashboard/src/views/AdminView.tsx
import { useState } from 'react';
import { Plus, Trash2 } from 'lucide-react';
import { Sidebar, SidebarBrand } from '@/ui/nav/Sidebar';
import { TopBar } from '@/ui/nav/TopBar';
import { OperatorClock } from '@/ui/nav/OperatorClock';
import { Button } from '@/ui/primitives/Button';
import { Panel } from '@/ui/primitives/Panel';
import { Stack } from '@/ui/primitives/Stack';
import { Chip } from '@/ui/primitives/Chip';
import { EnrollRoverWizard } from '@/ui/forms/EnrollRoverWizard';
import { DeleteRoverDrawer } from '@/ui/forms/DeleteRoverDrawer';
import { useFleet, isOnline, type FleetRover } from '@/state/useFleet';
import styles from './AdminView.module.css';

export function AdminView() {
  const [enrollOpen, setEnrollOpen] = useState(false);
  const [pendingDelete, setPendingDelete] = useState<FleetRover | null>(null);
  const { rovers, loading, refresh } = useFleet();

  return (
    <div className={styles.shell}>
      <SidebarBrand />
      <TopBar
        left={
          <div className={styles.title}>
            <h1>// ADMIN</h1>
            <div className={styles.sub}>Fleet management</div>
          </div>
        }
        right={<OperatorClock />}
      />
      <Sidebar />
      <main className={styles.body}>
        <section className={styles.content}>
          <Panel title="ENROLLMENT">
            <div className={styles.enrollRow}>
              <span className={styles.hint}>Generate an identity.toml for a new rover.</span>
              <Button variant="primary" icon={<Plus size={12} />} onClick={() => setEnrollOpen(true)}>
                Add rover
              </Button>
            </div>
          </Panel>

          <Panel
            title="ROVERS"
            note={loading ? 'loading…' : `${rovers.length} total`}
            style={{ marginTop: 12 }}
          >
            {rovers.length === 0 && !loading ? (
              <div className={styles.empty}>No rovers enrolled yet.</div>
            ) : (
              <div className={styles.roverList}>
                {rovers.map((r) => {
                  const online = isOnline(r.lastSeen);
                  return (
                    <div key={r.id} className={styles.roverRow}>
                      <Stack direction="column" gap={4} style={{ minWidth: 140 }}>
                        <span className={styles.roverId}>{r.id}</span>
                        <span className={styles.roverName}>{r.name}</span>
                      </Stack>
                      <Stack direction="row" gap={16} align="center" wrap>
                        <Chip tone={online ? 'default' : 'caution'} dot>
                          {online ? 'online' : 'offline'}
                        </Chip>
                        <Field label="Last seen" value={r.lastSeen ? new Date(r.lastSeen).toLocaleString() : '—'} />
                        <Field label="Image" value={r.imageVersion ?? '—'} />
                        <Field label="Role" value={r.role} />
                      </Stack>
                      <button
                        type="button"
                        className={styles.deleteBtn}
                        onClick={() => setPendingDelete(r)}
                        aria-label={`Delete rover ${r.id}`}
                        title="Delete rover"
                      >
                        <Trash2 size={14} />
                      </button>
                    </div>
                  );
                })}
              </div>
            )}
          </Panel>
        </section>

        <EnrollRoverWizard
          open={enrollOpen}
          onClose={() => {
            setEnrollOpen(false);
            refresh();
          }}
          onEnrolled={refresh}
        />
        <DeleteRoverDrawer
          open={pendingDelete !== null}
          rover={pendingDelete}
          onClose={() => setPendingDelete(null)}
          onDeleted={refresh}
        />
      </main>
    </div>
  );
}

function Field({ label, value }: { label: string; value: string }) {
  return (
    <div className={styles.field}>
      <div className={styles.fieldLabel}>{label}</div>
      <div className={styles.fieldValue}>{value}</div>
    </div>
  );
}
