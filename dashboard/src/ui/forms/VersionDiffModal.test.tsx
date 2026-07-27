import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { VersionDiffModal } from './VersionDiffModal';

describe('VersionDiffModal', () => {
  it('shows current, new, and rendered changelog markdown', () => {
    render(
      <VersionDiffModal
        open title="Update power on rover-01"
        currentVersion="1.4.2" newVersion="1.5.0"
        meta="cosign-verified"
        changelogMarkdown={'## Notes\n\n- thing'}
        applyLabel="Apply update" onApply={() => {}} onClose={() => {}}
      />,
    );
    expect(screen.getByText('1.4.2')).toBeInTheDocument();
    expect(screen.getByText('1.5.0')).toBeInTheDocument();
    expect(screen.getByText('thing')).toBeInTheDocument();
    expect(screen.getByText('cosign-verified')).toBeInTheDocument();
  });

  it('calls onApply', () => {
    const onApply = vi.fn();
    render(
      <VersionDiffModal open title="t" currentVersion="1" newVersion="2"
        changelogMarkdown="" applyLabel="Apply" onApply={onApply} onClose={() => {}} />,
    );
    fireEvent.click(screen.getByText('Apply'));
    expect(onApply).toHaveBeenCalledTimes(1);
  });

  it('renders the warning slot', () => {
    render(
      <VersionDiffModal open title="t" currentVersion="1" newVersion="2"
        changelogMarkdown="" applyLabel="Apply" onApply={() => {}} onClose={() => {}}
        warning={<span>reboot notice</span>} />,
    );
    expect(screen.getByText('reboot notice')).toBeInTheDocument();
  });
});
