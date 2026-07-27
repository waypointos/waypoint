// dashboard/src/lib/highlight/toml.ts
//
// Tiny line-based TOML lexer for CodeViewer. Recognizes section headers,
// key/value pairs, double-quoted strings, and comments. String values that
// appear inside the `[nats]` section are tagged as `secret` so the viewer
// can underline them.

export type TomlTokenKind = 'section' | 'key' | 'punct' | 'string' | 'secret' | 'comment';
export type TomlToken = { kind: TomlTokenKind; text: string };

const SECRET_SECTION = 'nats';

export function lexToml(input: string): TomlToken[][] {
  if (input === '') return [];
  const lines = input.split('\n');
  const out: TomlToken[][] = [];
  let currentSection = '';

  for (const raw of lines) {
    const tokens: TomlToken[] = [];
    if (raw.trim() === '') {
      out.push(tokens);
      continue;
    }
    if (raw.trimStart().startsWith('#')) {
      tokens.push({ kind: 'comment', text: raw });
      out.push(tokens);
      continue;
    }
    const secMatch = /^(\s*)(\[[^\]]+\])(\s*)(#.*)?$/.exec(raw);
    if (secMatch) {
      const [, leadWs, header, tailWs, comment] = secMatch;
      if (leadWs) tokens.push({ kind: 'punct', text: leadWs });
      tokens.push({ kind: 'section', text: header });
      currentSection = header.slice(1, -1);
      if (comment) {
        if (tailWs) tokens.push({ kind: 'punct', text: tailWs });
        tokens.push({ kind: 'comment', text: comment });
      } else if (tailWs && tailWs.length > 0) {
        tokens.push({ kind: 'punct', text: tailWs });
      }
      out.push(tokens);
      continue;
    }
    const kvMatch = /^(\s*)([A-Za-z_][\w-]*)(\s*=\s*)("[^"]*")(\s*)(#.*)?$/.exec(raw);
    if (kvMatch) {
      const [, leadWs, key, eq, value, tailWs, comment] = kvMatch;
      if (leadWs) tokens.push({ kind: 'punct', text: leadWs });
      tokens.push({ kind: 'key', text: key });
      tokens.push({ kind: 'punct', text: eq });
      tokens.push({
        kind: currentSection === SECRET_SECTION ? 'secret' : 'string',
        text: value,
      });
      if (comment) {
        if (tailWs) tokens.push({ kind: 'punct', text: tailWs });
        tokens.push({ kind: 'comment', text: comment });
      } else if (tailWs && tailWs.length > 0) {
        tokens.push({ kind: 'punct', text: tailWs });
      }
      out.push(tokens);
      continue;
    }
    tokens.push({ kind: 'punct', text: raw });
    out.push(tokens);
  }
  return out;
}
