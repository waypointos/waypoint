import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/react';
import { ModuleIframe } from './ModuleIframe';

describe('ModuleIframe', () => {
  it('renders an iframe pointing at /module/<id>/', () => {
    const { container } = render(<ModuleIframe moduleId="power-monitor" />);
    const iframe = container.querySelector('iframe');
    expect(iframe).not.toBeNull();
    expect(iframe!.getAttribute('src')).toBe('/module/power-monitor/');
    expect(iframe!.getAttribute('sandbox')).toContain('allow-scripts');
    expect(iframe!.getAttribute('sandbox')).toContain('allow-same-origin');
  });
});
