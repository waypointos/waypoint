import { describe, it, expect } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { LocationCard } from './LocationCard';

describe('LocationCard', () => {
  it('shows the Paris no-fix state when there is no position', () => {
    render(<LocationCard roverId="demo" position={null} />);
    expect(screen.getByText(/LOCATION/)).toBeInTheDocument();
    expect(screen.getByText(/NO FIX · PARIS/)).toBeInTheDocument();
  });

  it('renders coordinates when a fix is present', () => {
    render(<LocationCard roverId="demo" position={{ lat: 48.8566, lng: 2.3522 }} />);
    expect(screen.getByText(/48\.8566, 2\.3522/)).toBeInTheDocument();
  });

  it('opens the borderless 3D map dialog from the expand icon', () => {
    render(<LocationCard roverId="demo" position={null} />);
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(screen.getAllByTestId('location-map')).toHaveLength(1);
    fireEvent.click(screen.getByRole('button', { name: /open 3d map/i }));
    // Bare dialog has no visible title; it's labelled for a11y instead.
    expect(screen.getByRole('dialog', { name: /map · 3d/i })).toBeInTheDocument();
    expect(screen.getAllByTestId('location-map')).toHaveLength(2);
  });
});
