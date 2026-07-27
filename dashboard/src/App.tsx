// dashboard/src/App.tsx
import { useEffect } from 'react';
import { Router } from './router';
import { requireLogin } from './state/auth';

export function App() {
  useEffect(() => { requireLogin(); }, []);
  return <Router />;
}
