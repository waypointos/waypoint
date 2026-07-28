import { describe, it, expect } from 'vitest';
import {
  unwrapModuleConfig, wrapModuleConfig, parseFlatConfig, buildFlatConfig,
} from './moduleConfigToml';

const umrFields = [
  { key: 'host', type: 'url' },
  { key: 'password', type: 'password' },
  { key: 'poll_interval_s', type: 'number' },
];

describe('parseFlatConfig', () => {
  it('reads quoted and bare values', () => {
    const got = parseFlatConfig('host = "https://192.168.105.1"\npoll_interval_s = 5\n');
    expect(got).toEqual({ host: 'https://192.168.105.1', poll_interval_s: '5' });
  });

  it('unescapes quotes inside a string', () => {
    expect(parseFlatConfig('password = "a\\"b"\n')).toEqual({ password: 'a"b' });
  });

  it('ignores comments and blanks', () => {
    expect(parseFlatConfig('# a comment\n\nhost = "h"\n')).toEqual({ host: 'h' });
  });
});

describe('buildFlatConfig', () => {
  it('quotes strings and leaves numbers bare, in manifest order', () => {
    const out = buildFlatConfig(
      { poll_interval_s: '5', host: 'https://10.0.0.1', password: 'pw' },
      umrFields,
    );
    expect(out).toBe('host = "https://10.0.0.1"\npassword = "pw"\npoll_interval_s = 5\n');
  });

  it('omits empty fields so the module keeps its own default', () => {
    expect(buildFlatConfig({ host: '', password: 'pw' }, umrFields)).toBe('password = "pw"\n');
  });

  it('escapes quotes and backslashes', () => {
    expect(buildFlatConfig({ password: 'a"b\\c' }, umrFields)).toBe('password = "a\\"b\\\\c"\n');
  });

  it('quotes a non-numeric value declared as a number rather than emitting invalid TOML', () => {
    expect(buildFlatConfig({ poll_interval_s: 'fast' }, umrFields)).toBe('poll_interval_s = "fast"\n');
  });

  it('keeps keys and comments the schema does not model', () => {
    const original = '# hand-written\nhost = "old"\nundocumented_flag = "keep me"\n';
    const out = buildFlatConfig({ host: 'new' }, umrFields, original);
    expect(out).toBe('host = "new"\n# hand-written\nundocumented_flag = "keep me"\n');
  });

  it('returns empty when nothing is set, which clears the config', () => {
    expect(buildFlatConfig({}, umrFields)).toBe('');
  });

  it('round-trips through the table wrapper', () => {
    const flat = buildFlatConfig({ host: 'https://h', poll_interval_s: '5' }, umrFields);
    const stored = wrapModuleConfig(flat, 'umr');
    expect(parseFlatConfig(unwrapModuleConfig(stored, 'umr'))).toEqual({
      host: 'https://h', poll_interval_s: '5',
    });
  });
});

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
