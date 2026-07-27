import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter, Routes, Route } from 'react-router-dom';
import { TabStrip } from './TabStrip';

const tabs = [
  { id: 'overview',   label: 'OVERVIEW',   to: '/rover/r1/overview' },
  { id: 'control',    label: 'CONTROL',    to: '/rover/r1/control' },
  { id: 'connection', label: 'CONNECTION', to: '/rover/r1/connection' },
  { id: 'logs',       label: 'LOGS',       to: '/rover/r1/logs' },
];

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/rover/:id/:tab" element={<TabStrip tabs={tabs} />} />
      </Routes>
    </MemoryRouter>
  );
}

describe('TabStrip', () => {
  it('renders every tab label', () => {
    renderAt('/rover/r1/overview');
    for (const t of tabs) {
      expect(screen.getByText(t.label)).toBeInTheDocument();
    }
  });

  it('marks the active tab via aria-selected based on URL', () => {
    renderAt('/rover/r1/control');
    const active = screen.getByRole('tab', { selected: true });
    expect(active).toHaveTextContent('CONTROL');
  });

  it('has role="tablist" with role="tab" children', () => {
    renderAt('/rover/r1/overview');
    expect(screen.getByRole('tablist')).toBeInTheDocument();
    expect(screen.getAllByRole('tab')).toHaveLength(tabs.length);
  });
});
