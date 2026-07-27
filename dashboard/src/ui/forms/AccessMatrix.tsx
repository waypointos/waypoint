// dashboard/src/ui/forms/AccessMatrix.tsx
//
// Two-pane access editor. Left pane: searchable list of users with a
// "n of m rovers" summary chip. Right pane: for the selected user, every
// rover is rendered as a row with the 4-button segmented role control
// inline. Both panes scroll independently, so this scales to dozens of
// users and dozens of rovers without horizontal-scroll pain.
//
// Mutation flow: clicking a segment calls onChange, which is expected to
// grant or revoke and then refetch the user list. Per-row busy state is
// local to keep the segment locked while the request is in flight.
import { useEffect, useMemo, useState } from 'react';
import { Loader2, Search } from 'lucide-react';
import { Chip } from '../primitives/Chip';
import type { AdminUser, CellRole, Role } from '@/state/useAdminUsers';
import type { FleetRover } from '@/state/useFleet';
import { isOnline } from '@/state/useFleet';
import styles from './AccessMatrix.module.css';

type Props = {
  users: AdminUser[];
  rovers: FleetRover[];
  /** Called when a row's role changes. Pass 'none' to mean revoke. */
  onChange?: (
    userId: string,
    roverId: string,
    role: CellRole,
  ) => Promise<void>;
};

type RoleDef = { value: CellRole; short: string; long: string };
const ROLES: RoleDef[] = [
  { value: 'none',    short: '—',   long: 'No access' },
  { value: 'monitor', short: 'MON', long: 'Monitor — read-only telemetry' },
  { value: 'control', short: 'CTL', long: 'Control — issue commands' },
  { value: 'admin',   short: 'ADM', long: 'Admin — full rover admin' },
];

type RowKey = string;
function rowKey(userId: string, roverId: string): RowKey {
  return `${userId}::${roverId}`;
}

function currentRole(user: AdminUser, roverId: string): CellRole {
  const g = user.access.find((a) => a.roverId === roverId);
  return g ? g.role : 'none';
}

export function AccessMatrix({ users, rovers, onChange }: Props) {
  const [userQuery, setUserQuery] = useState('');
  const [roverQuery, setRoverQuery] = useState('');
  const [selectedId, setSelectedId] = useState<string | null>(
    users[0]?.id ?? null,
  );
  const [busy, setBusy] = useState<Record<RowKey, boolean>>({});
  const [err, setErr]   = useState<Record<RowKey, string | undefined>>({});

  // Reselect a sane default when the user list changes underneath us
  // (refresh after invite, refresh after delete, etc.).
  useEffect(() => {
    if (users.length === 0) {
      setSelectedId(null);
      return;
    }
    if (!selectedId || !users.some((u) => u.id === selectedId)) {
      setSelectedId(users[0].id);
    }
  }, [users, selectedId]);

  const filteredUsers = useMemo(() => {
    const q = userQuery.trim().toLowerCase();
    const list = q
      ? users.filter((u) => u.email.toLowerCase().includes(q))
      : users;
    // Sort: admins first, then alphabetical email.
    return [...list].sort((a, b) => {
      if (a.isAdmin !== b.isAdmin) return a.isAdmin ? -1 : 1;
      return a.email.localeCompare(b.email);
    });
  }, [users, userQuery]);

  const selected = useMemo(
    () => users.find((u) => u.id === selectedId) ?? null,
    [users, selectedId],
  );

  const filteredRovers = useMemo(() => {
    const q = roverQuery.trim().toLowerCase();
    if (!q) return rovers;
    return rovers.filter(
      (r) =>
        r.id.toLowerCase().includes(q) || r.name.toLowerCase().includes(q),
    );
  }, [rovers, roverQuery]);

  if (rovers.length === 0) {
    return (
      <div className={styles.empty}>
        <span className={styles.naLabel}>N/A</span>
        <span className={styles.naHint}>
          No rovers enrolled — enroll one in Admin first.
        </span>
      </div>
    );
  }

  async function handleChange(roverId: string, role: CellRole) {
    if (!onChange || !selected) return;
    const k = rowKey(selected.id, roverId);
    setBusy((b) => ({ ...b, [k]: true }));
    setErr((e) => ({ ...e, [k]: undefined }));
    try {
      await onChange(selected.id, roverId, role);
    } catch (ex) {
      setErr((e) => ({
        ...e,
        [k]: ex instanceof Error ? ex.message : 'failed',
      }));
    } finally {
      setBusy((b) => ({ ...b, [k]: false }));
    }
  }

  return (
    <div className={styles.editor}>
      <UserList
        users={filteredUsers}
        roverCount={rovers.length}
        selectedId={selectedId}
        onSelect={setSelectedId}
        query={userQuery}
        onQueryChange={setUserQuery}
      />
      <RoverList
        selected={selected}
        rovers={filteredRovers}
        roverQuery={roverQuery}
        onRoverQueryChange={setRoverQuery}
        busy={busy}
        err={err}
        onChange={handleChange}
      />
    </div>
  );
}

// ── User list (left pane) ────────────────────────────────────────────────────

function UserList({
  users,
  roverCount,
  selectedId,
  onSelect,
  query,
  onQueryChange,
}: {
  users: AdminUser[];
  roverCount: number;
  selectedId: string | null;
  onSelect: (id: string) => void;
  query: string;
  onQueryChange: (q: string) => void;
}) {
  return (
    <aside className={styles.usersPane} aria-label="Users">
      <div className={styles.searchRow}>
        <Search size={12} className={styles.searchIcon} aria-hidden />
        <input
          type="text"
          className={styles.search}
          placeholder="Search users…"
          value={query}
          onChange={(e) => onQueryChange(e.target.value)}
        />
      </div>
      <ul className={styles.userList} role="listbox" aria-label="Users">
        {users.length === 0 ? (
          <li className={styles.userEmpty}>No matching users.</li>
        ) : (
          users.map((u) => {
            const grants = u.access.length;
            const isSel = u.id === selectedId;
            return (
              <li key={u.id}>
                <button
                  type="button"
                  role="option"
                  aria-selected={isSel}
                  className={[styles.userBtn, isSel && styles.userBtnActive]
                    .filter(Boolean)
                    .join(' ')}
                  onClick={() => onSelect(u.id)}
                >
                  <span className={styles.userEmail} title={u.email}>
                    {u.email}
                  </span>
                  <span className={styles.userMeta}>
                    {u.isAdmin ? (
                      <Chip tone="caution">Admin</Chip>
                    ) : null}
                    <span className={styles.userCount}>
                      {grants}/{roverCount}
                    </span>
                  </span>
                </button>
              </li>
            );
          })
        )}
      </ul>
    </aside>
  );
}

// ── Rover list (right pane) ──────────────────────────────────────────────────

function RoverList({
  selected,
  rovers,
  roverQuery,
  onRoverQueryChange,
  busy,
  err,
  onChange,
}: {
  selected: AdminUser | null;
  rovers: FleetRover[];
  roverQuery: string;
  onRoverQueryChange: (q: string) => void;
  busy: Record<RowKey, boolean>;
  err: Record<RowKey, string | undefined>;
  onChange: (roverId: string, role: CellRole) => Promise<void>;
}) {
  if (!selected) {
    return (
      <section className={styles.detailPane}>
        <div className={styles.detailEmpty}>
          <span className={styles.naLabel}>N/A</span>
          <span className={styles.naHint}>Select a user to edit access.</span>
        </div>
      </section>
    );
  }

  return (
    <section className={styles.detailPane} aria-label={`Access for ${selected.email}`}>
      <header className={styles.detailHead}>
        <div className={styles.detailTitleRow}>
          <span className={styles.detailEyebrow}>ACCESS FOR</span>
          <span className={styles.detailTitle}>{selected.email}</span>
          {selected.isAdmin ? <Chip tone="caution">Admin</Chip> : null}
        </div>
        <div className={styles.detailSub}>
          {selected.access.length} of {rovers.length} rovers granted
          {selected.lastLoginAt
            ? ` · last login ${new Date(selected.lastLoginAt).toLocaleString()}`
            : ''}
        </div>
      </header>

      <div className={styles.searchRow}>
        <Search size={12} className={styles.searchIcon} aria-hidden />
        <input
          type="text"
          className={styles.search}
          placeholder="Search rovers…"
          value={roverQuery}
          onChange={(e) => onRoverQueryChange(e.target.value)}
        />
      </div>

      <ul className={styles.roverList}>
        {rovers.length === 0 ? (
          <li className={styles.detailEmpty}>
            <span className={styles.naHint}>No matching rovers.</span>
          </li>
        ) : (
          rovers.map((r) => {
            const role = currentRole(selected, r.id);
            const k = rowKey(selected.id, r.id);
            const isBusy = !!busy[k];
            const cellErr = err[k];
            const online = isOnline(r.lastSeen);
            return (
              <li key={r.id} className={styles.roverRow}>
                <div className={styles.roverInfo}>
                  <span className={styles.roverId}>{r.id}</span>
                  <span className={styles.roverName}>{r.name}</span>
                </div>
                <span
                  className={[styles.roverStatus, online ? styles.online : styles.offline]
                    .filter(Boolean)
                    .join(' ')}
                  title={online ? 'online' : 'offline'}
                >
                  <span className={styles.statusDot} />
                  {online ? 'online' : 'offline'}
                </span>
                <div className={styles.roverActions}>
                  <RoleSegments
                    value={role}
                    disabled={isBusy}
                    onChange={(next) => onChange(r.id, next as Role | 'none')}
                    ariaLabel={`Access for ${selected.email} on ${r.id}`}
                  />
                  {isBusy ? (
                    <Loader2 size={12} className={styles.spinner} aria-hidden />
                  ) : null}
                </div>
                {cellErr ? (
                  <div className={styles.rowErr} title={cellErr}>
                    {cellErr}
                  </div>
                ) : null}
              </li>
            );
          })
        )}
      </ul>
    </section>
  );
}

// ── Segmented role control ───────────────────────────────────────────────────

function RoleSegments({
  value,
  disabled,
  onChange,
  ariaLabel,
}: {
  value: CellRole;
  disabled: boolean;
  onChange: (next: CellRole) => void;
  ariaLabel: string;
}) {
  return (
    <div className={styles.segments} role="radiogroup" aria-label={ariaLabel}>
      {ROLES.map((opt) => {
        const active = opt.value === value;
        return (
          <button
            key={opt.value}
            type="button"
            role="radio"
            aria-checked={active}
            disabled={disabled}
            className={[
              styles.segment,
              active && styles.segmentActive,
              opt.value === 'admin' && active && styles.segmentAdmin,
            ].filter(Boolean).join(' ')}
            onClick={() => { if (!active) onChange(opt.value); }}
            title={opt.long}
          >
            {opt.short}
          </button>
        );
      })}
    </div>
  );
}
