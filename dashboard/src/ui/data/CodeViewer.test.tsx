import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { CodeViewer } from './CodeViewer';

const TOML = '[rover]\nid = "sim-02"';

describe('CodeViewer', () => {
  it('renders bracketed title, meta, and body content', () => {
    render(
      <CodeViewer
        filename="identity.toml"
        content={TOML}
        lang="toml"
        meta="2 LINES · 24 B"
      />,
    );
    expect(screen.getByText('IDENTITY.TOML')).toBeInTheDocument();
    expect(screen.getByText('2 LINES · 24 B')).toBeInTheDocument();
    expect(screen.getByTestId('code-viewer-body').textContent).toContain('id = "sim-02"');
  });

  it('renders an optional status pip with tone class', () => {
    render(
      <CodeViewer
        filename="identity.toml"
        content={TOML}
        lang="toml"
        status={{ label: 'GENERATED', tone: 'ok' }}
      />,
    );
    const pip = screen.getByText('GENERATED');
    expect(pip).toBeInTheDocument();
    expect(pip.className).toMatch(/ok/);
  });

  it('invokes onCopy when the copy button is clicked', () => {
    const onCopy = vi.fn();
    render(
      <CodeViewer
        filename="identity.toml"
        content={TOML}
        lang="toml"
        actions={{ onCopy }}
      />,
    );
    fireEvent.click(screen.getByLabelText('Copy'));
    expect(onCopy).toHaveBeenCalledWith(TOML);
  });

  it('invokes onDownload when the download button is clicked', () => {
    const onDownload = vi.fn();
    render(
      <CodeViewer
        filename="identity.toml"
        content={TOML}
        lang="toml"
        actions={{ onDownload }}
      />,
    );
    fireEvent.click(screen.getByLabelText('Download'));
    expect(onDownload).toHaveBeenCalledWith('identity.toml', TOML);
  });

  it('suppresses the body when body={false}', () => {
    render(
      <CodeViewer
        filename="identity.toml"
        content={TOML}
        lang="toml"
        body={false}
        meta="2 LINES · 24 B"
      />,
    );
    expect(screen.queryByTestId('code-viewer-body')).toBeNull();
    expect(screen.getByText('IDENTITY.TOML')).toBeInTheDocument();
  });

  it('renders no action buttons or actions row when actions is empty', () => {
    render(
      <CodeViewer
        filename="identity.toml"
        content={TOML}
        lang="toml"
        actions={{}}
      />,
    );
    expect(screen.queryByLabelText('Copy')).toBeNull();
    expect(screen.queryByLabelText('Download')).toBeNull();
  });

  it('renders a footer note when provided', () => {
    render(
      <CodeViewer
        filename="identity.toml"
        content={TOML}
        lang="toml"
        footerNote="destination · ~/data/waypoint/identity.toml"
      />,
    );
    expect(screen.getByText(/~\/data\/waypoint\/identity\.toml/)).toBeInTheDocument();
  });
});
