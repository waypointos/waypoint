import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { ExportDialog } from './ExportDialog';
import type { Sample } from '../../lib/episode/decoders';

const S = (tNs: bigint, value: number | null): Sample => ({ tNs, value });

const series = new Map<string, Sample[]>([
  ['module.drill.sensor.state.cell_a_g', [S(1_000_000_000n, 100), S(2_000_000_000n, 110)]],
  ['telemetry.motors.motors.0.speed', [S(1_000_000_000n, 0.5)]],
]);

describe('ExportDialog', () => {
  it('exports visible series over the selection range as <id>.csv', () => {
    const download = vi.fn();
    render(
      <ExportDialog
        open
        onClose={() => {}}
        series={series}
        visibleIds={['module.drill.sensor.state.cell_a_g']}
        range={{ startNs: 1_000_000_000n, endNs: 2_000_000_000n }}
        selection={{ startNs: 1_500_000_000n, endNs: 2_000_000_000n }}
        episodeId="ep-1"
        download={download}
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: /export/i }));
    expect(download).toHaveBeenCalledTimes(1);
    const [filename, csv] = download.mock.calls[0] as [string, string];
    expect(filename).toBe('ep-1.csv');
    const lines = csv.trimEnd().split('\n');
    expect(lines[0]).toBe('time_ns,time_iso,module.drill.sensor.state.cell_a_g');
    expect(lines).toHaveLength(2); // only the 2s sample is inside the selection
    expect(lines[1].startsWith('2000000000,')).toBe(true);
  });

  it('lets unchecked series drop out of the export', () => {
    const download = vi.fn();
    render(
      <ExportDialog
        open onClose={() => {}} series={series}
        visibleIds={['module.drill.sensor.state.cell_a_g', 'telemetry.motors.motors.0.speed']}
        range={{ startNs: 0n, endNs: 9_000_000_000n }} selection={null}
        episodeId="ep-1" download={download}
      />,
    );
    fireEvent.click(screen.getByLabelText('telemetry.motors.motors.0.speed'));
    fireEvent.click(screen.getByRole('button', { name: /export/i }));
    const [, csv] = download.mock.calls[0] as [string, string];
    expect(csv.split('\n')[0]).toBe('time_ns,time_iso,module.drill.sensor.state.cell_a_g');
  });
});
