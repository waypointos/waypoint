import { describe, expect, it } from 'vitest';
import { lexToml, type TomlToken } from './toml';

describe('lexToml', () => {
  it('returns empty array for empty input', () => {
    expect(lexToml('')).toEqual([]);
  });

  it('lexes a section header', () => {
    expect(lexToml('[rover]')).toEqual([
      [{ kind: 'section', text: '[rover]' }],
    ]);
  });

  it('lexes a key-value line with a string', () => {
    expect(lexToml('id = "sim-02"')).toEqual([
      [
        { kind: 'key',    text: 'id' },
        { kind: 'punct',  text: ' = ' },
        { kind: 'string', text: '"sim-02"' },
      ],
    ]);
  });

  it('preserves whitespace inside the key column', () => {
    expect(lexToml('name   = "Recon"')).toEqual([
      [
        { kind: 'key',    text: 'name' },
        { kind: 'punct',  text: '   = ' },
        { kind: 'string', text: '"Recon"' },
      ],
    ]);
  });

  it('lexes a comment line', () => {
    expect(lexToml('# secret credential — do not share')).toEqual([
      [{ kind: 'comment', text: '# secret credential — do not share' }],
    ]);
  });

  it('lexes a trailing comment', () => {
    expect(lexToml('[nats] # secret')).toEqual([
      [
        { kind: 'section', text: '[nats]' },
        { kind: 'punct',   text: ' ' },
        { kind: 'comment', text: '# secret' },
      ],
    ]);
  });

  it('preserves blank lines as empty token arrays', () => {
    expect(lexToml('[a]\n\n[b]')).toEqual([
      [{ kind: 'section', text: '[a]' }],
      [],
      [{ kind: 'section', text: '[b]' }],
    ]);
  });

  it('marks string tokens inside [nats] as secret', () => {
    const out = lexToml('[nats]\naccount_jwt = "eyJ..."');
    expect(out[1]).toEqual([
      { kind: 'key',    text: 'account_jwt' },
      { kind: 'punct',  text: ' = ' },
      { kind: 'secret', text: '"eyJ..."' },
    ]);
  });

  it('does not mark strings outside [nats] as secret', () => {
    const out = lexToml('[rover]\nid = "sim-02"');
    expect(out[1].some((t: TomlToken) => t.kind === 'secret')).toBe(false);
  });

  it('handles the full identity.toml shape', () => {
    const input = [
      '[rover]',
      'id   = "sim-02"',
      'name = "Recon"',
      '',
      '[proxy]',
      'url             = "wss://example/leaf"',
      'operator_pubkey = "OAOPS5..."',
      '',
      '[nats]',
      'account_jwt = "eyJ..."',
      'user_seed   = "SUAAB..."',
    ].join('\n');
    const out = lexToml(input);
    expect(out.length).toBe(11);
    const lastTokens = out[out.length - 1];
    expect(lastTokens.find((t) => t.kind === 'secret')).toBeTruthy();
    // [proxy] strings must NOT be tagged as secret
    expect(out[5].some((t: TomlToken) => t.kind === 'secret')).toBeFalsy();
    expect(out[6].some((t: TomlToken) => t.kind === 'secret')).toBeFalsy();
  });
});
