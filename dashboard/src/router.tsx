// dashboard/src/router.tsx
import { lazy, Suspense } from 'react';
import { createBrowserRouter, RouterProvider, Navigate } from 'react-router-dom';
import { useMe } from './state/mode';
import { Gallery } from './ui/__gallery__/Gallery';
import { FleetView } from './views/FleetView';
import { MapView } from './views/MapView';
import { RoverView } from './views/RoverView/RoverView';
import { TeleopView } from './views/Teleop/TeleopView';
import { AdminView } from './views/AdminView';
import { AuditLogView } from './views/AuditLogView';
import { AdminUsersView } from './views/AdminUsersView';
import { AdminModulesView } from './views/AdminModulesView';
import { ComingSoonView } from './views/ComingSoonView';
import { ModulesView } from './views/ModulesView/ModulesView';
import { ReleasesView } from './views/ReleasesView/ReleasesView';

// Split out so the MCAP parser and its zstd decoder stay off every other route.
const EpisodePlayerView = lazy(() =>
  import('./views/EpisodePlayer/EpisodePlayerView')
    .then((m) => ({ default: m.EpisodePlayerView })));

function AdminRoute() {
  const me = useMe();
  // While /api/me is in-flight, render nothing (same as FleetView's initial null).
  if (!me) return null;
  if (!me.isAdmin) return <Navigate to="/" replace />;
  return <AdminView />;
}

function AuditRoute() {
  const me = useMe();
  if (!me) return null;
  // Audit is admin-only at the top level; per-rover non-admin views can mount
  // AuditLogView directly with a rover preselected (future task).
  if (!me.isAdmin) return <Navigate to="/" replace />;
  return <AuditLogView />;
}

function AdminUsersRoute() {
  const me = useMe();
  if (!me) return null;
  if (!me.isAdmin) return <Navigate to="/" replace />;
  return <AdminUsersView />;
}

function AdminModulesRoute() {
  const me = useMe();
  if (!me) return null;
  if (!me.isAdmin) return <Navigate to="/" replace />;
  return <AdminModulesView />;
}

const router = createBrowserRouter([
  { path: '/',            element: <FleetView /> },
  { path: '/map',         element: <MapView /> },
  { path: '/rover/:id',        element: <RoverView /> },
  { path: '/rover/:id/teleop', element: <TeleopView /> },
  {
    path: '/rover/:id/episodes/:episodeId',
    element: <Suspense fallback={null}><EpisodePlayerView /></Suspense>,
  },
  { path: '/rover/:id/:tab',   element: <RoverView /> },
  { path: '/modules',     element: <ModulesView /> },
  { path: '/releases',    element: <ReleasesView /> },
  { path: '/settings',    element: <ComingSoonView title="SETTINGS" subtitle="Workspace settings" hint="Workspace preferences and integrations will live here." /> },
  { path: '/admin',       element: <AdminRoute /> },
  { path: '/admin/audit', element: <AuditRoute /> },
  { path: '/admin/users', element: <AdminUsersRoute /> },
  { path: '/admin/modules', element: <AdminModulesRoute /> },
  { path: '/ui-gallery',  element: <Gallery /> },
  { path: '*',            element: <FleetView /> },
]);

export function Router() {
  return <RouterProvider router={router} />;
}
