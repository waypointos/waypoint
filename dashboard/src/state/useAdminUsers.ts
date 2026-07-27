// dashboard/src/state/useAdminUsers.ts
//
// Admin-only user directory backed by GET /api/users.
// Also exposes invite + grant/revoke helpers. Mutations don't hold local
// state — callers invoke `refresh()` after a mutation to re-pull the list.
//
// The grant/revoke request shape mirrors `proxy/internal/api/access.go`:
//   POST   /api/rovers/{id}/access   {"userId":"<uuid>","role":"monitor|control|admin"}
//   DELETE /api/rovers/{id}/access/{userId}
import { useEffect, useState } from 'react';

export type Role = 'monitor' | 'control' | 'admin';
export type CellRole = 'none' | Role;

export type UserGrant = { roverId: string; role: Role };

export type AdminUser = {
  id: string;
  email: string;
  isAdmin: boolean;
  createdAt: string;
  lastLoginAt?: string;
  access: UserGrant[];
};

export type InviteResult = {
  email: string;
  acceptUrl?: string;
  emailSent: boolean;
};

export function useAdminUsers() {
  const [users, setUsers] = useState<AdminUser[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const refresh = async () => {
    try {
      setLoading(true);
      const r = await fetch('/api/users', { credentials: 'include' });
      if (!r.ok) throw new Error(`/api/users -> ${r.status}`);
      const data = (await r.json()) as AdminUser[] | null;
      setUsers(data ?? []);
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return { users, loading, error, refresh };
}

export async function inviteUser(email: string): Promise<InviteResult> {
  const r = await fetch('/api/users/invite', {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email }),
  });
  if (!r.ok) {
    throw new Error(`/api/users/invite -> ${r.status} ${await r.text()}`);
  }
  return (await r.json()) as InviteResult;
}

export async function grantAccess(
  roverId: string,
  userId: string,
  role: Role,
): Promise<void> {
  const r = await fetch(`/api/rovers/${roverId}/access`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ userId, role }),
  });
  if (!r.ok) {
    throw new Error(`grant -> ${r.status} ${await r.text()}`);
  }
}

export async function revokeAccess(
  roverId: string,
  userId: string,
): Promise<void> {
  const r = await fetch(`/api/rovers/${roverId}/access/${userId}`, {
    method: 'DELETE',
    credentials: 'include',
  });
  if (!r.ok) {
    throw new Error(`revoke -> ${r.status} ${await r.text()}`);
  }
}
