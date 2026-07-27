import { describe, it, expect } from 'vitest';
import { unwrapModuleConfig, wrapModuleConfig } from './moduleConfigToml';

describe('unwrapModuleConfig', () => {
  it('strips the module table header', () => {
    const stored = '[modules_config.umr]\nhost = "https://192.168.105.1"\npassword = "x"\n';
    expect(unwrapModuleConfig(stored, 'umr')).toBe('host = "https://192.168.105.1"\npassword = "x"\n');
  });

  it('leaves a document with several tables alone', () => {
    const stored = '[[modules]]\nid = "umr"\n\n[modules_config.umr]\nhost = "h"\n';
    expect(unwrapModuleConfig(stored, 'umr')).toBe(stored);
  });

  it('leaves another module’s table alone', () => {
    const stored = '[modules_config.drill]\nhost = "h"\n';
    expect(unwrapModuleConfig(stored, 'umr')).toBe(stored);
  });

  it('passes through an empty config', () => {
    expect(unwrapModuleConfig('', 'umr')).toBe('');
  });
});

describe('wrapModuleConfig', () => {
  it('wraps flat keys in the module table', () => {
    expect(wrapModuleConfig('host = "h"\npassword = "x"\n', 'umr')).toBe(
      '[modules_config.umr]\nhost = "h"\npassword = "x"\n',
    );
  });

  it('keeps an already-wrapped document as typed', () => {
    const doc = '[modules_config.umr]\nhost = "h"\n';
    expect(wrapModuleConfig(doc, 'umr')).toBe(doc);
  });

  it('sends blank config as empty, not as a bare table header', () => {
    expect(wrapModuleConfig('   \n', 'umr')).toBe('');
  });

  it('round-trips', () => {
    const flat = 'host = "https://192.168.105.1"\npoll_interval_s = 5\n';
    expect(unwrapModuleConfig(wrapModuleConfig(flat, 'umr'), 'umr')).toBe(flat);
  });
});
