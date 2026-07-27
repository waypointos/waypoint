import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { SpeedReadout } from './SpeedReadout';

describe('SpeedReadout', () => {
  it('renders a measured speed to 2 decimals', () => {
    render(<SpeedReadout mps={0.371} preset="cruise" cap={0.45} />);
    expect(screen.getByText('0.37')).toBeInTheDocument();
    expect(screen.getByText(/cruise/i)).toBeInTheDocument();
  });

  it('shows N/A when speed is null', () => {
    render(<SpeedReadout mps={null} preset="slow" cap={1} />);
    expect(screen.getByText('N/A')).toBeInTheDocument();
  });
});
