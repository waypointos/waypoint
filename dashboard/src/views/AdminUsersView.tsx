// dashboard/src/views/AdminUsersView.tsx
//
// /admin/users — admin-only directory + access editor. Composed of two
// Panels (INVITE, ACCESS) inside the standard Sidebar + TopBar shell.
import { useState } from 'react';
import { Plus } from 'lucide-react';
import { Sidebar, SidebarBrand } from '@/ui/nav/Sidebar';
import { TopBar } from '@/ui/nav/TopBar';
import { OperatorClock } from '@/ui/nav/OperatorClock';
import { Button } from '@/ui/primitives/Button';
import { Panel } from '@/ui/primitives/Panel';
import { InviteUserDialog } from '@/ui/forms/InviteUserDialog';
import { AccessMatrix } from '@/ui/forms/AccessMatrix';
import {
  useAdminUsers,
  grantAccess,
  revokeAccess,
  type CellRole,
} from '@/state/useAdminUsers';
import { useFleet } from '@/state/useFleet';
import styles from './AdminUsersView.module.css';

export function AdminUsersView() {
  const { users, loading, error, refresh } = useAdminUsers();
  const { rovers } = useFleet();
  const [inviteOpen, setInviteOpen] = useState(false);

  const accessNote = loading
    ? 'loading…'
    : error
      ? `error: ${error}`
      : `${users.length} user${users.length === 1 ? '' : 's'}`;

  async function handleCellChange(
    userId: string,
    roverId: string,
    role: CellRole,
  ) {
    if (role === 'none') {
      await revokeAccess(roverId, userId);
    } else {
      await grantAccess(roverId, userId, role);
    }
    // Refetch after mutation rather than maintaining optimistic state.
    await refresh();
  }

  return (
    <div className={styles.shell}>
      <SidebarBrand />
      <TopBar
        left={
          <div className={styles.title}>
            <h1>// USERS</h1>
            <div className={styles.sub}>User management</div>
          </div>
        }
        right={<OperatorClock />}
      />
      <Sidebar />
      <main className={styles.body}>
        <section className={styles.content}>
          <Panel title="INVITE">
            <div className={styles.inviteRow}>
              <span className={styles.hint}>
                Provision access for a new member via email invitation.
              </span>
              <Button
                variant="primary"
                icon={<Plus size={12} />}
                onClick={() => setInviteOpen(true)}
              >
                Invite user
              </Button>
            </div>
          </Panel>

          <Panel
            title="ACCESS"
            note={accessNote}
            style={{ marginTop: 12 }}
          >
            {loading && users.length === 0 ? (
              <div className={styles.empty}>
                <span className={styles.naLabel}>N/A</span>
                <span className={styles.naHint}>fetching users…</span>
              </div>
            ) : error ? (
              <div className={styles.empty}>
                <span className={styles.naLabel}>N/A</span>
                <span className={styles.naHint}>unable to load users</span>
              </div>
            ) : users.length === 0 ? (
              <div className={styles.empty}>
                <span className={styles.naLabel}>N/A</span>
                <span className={styles.naHint}>
                  No users yet — invite the first one via the button above.
                </span>
              </div>
            ) : (
              <AccessMatrix
                users={users}
                rovers={rovers}
                onChange={handleCellChange}
              />
            )}
          </Panel>
        </section>

        <InviteUserDialog
          open={inviteOpen}
          onClose={() => setInviteOpen(false)}
          onInvited={refresh}
        />
      </main>
    </div>
  );
}
