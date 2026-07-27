// dashboard/src/ui/data/Metric.test.tsx
import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { Metric } from './Metric';

describe('Metric', () => {
  it('renders value + unit when value present', () => {
    render(<Metric label="Battery" value={87} unit="%" />);
    expect(screen.getByText('87')).toBeInTheDocument();
    expect(screen.getByText('%')).toBeInTheDocument();
  });

  it('renders muted N/A with reason hint when value is null', () => {
    render(<Metric label="Battery" value={null} naReason="no fuel-gauge module" />);
    expect(screen.getByText('N/A')).toBeInTheDocument();
    expect(screen.getByText('no fuel-gauge module')).toBeInTheDocument();
  });

  it('renders muted N/A with reason hint when value is undefined', () => {
    render(<Metric label="Heading" value={undefined} naReason="no IMU" />);
    expect(screen.getByText('N/A')).toBeInTheDocument();
    expect(screen.getByText('no IMU')).toBeInTheDocument();
  });
});
