// A module documents its config as flat keys, but the agent reads the same keys
// out of a [modules_config.<id>] table (agent/internal/modules/config.go). The
// form shows the flat schema and these convert at the API boundary.

function header(moduleId: string): string {
  return `[modules_config.${moduleId}]`;
}

function isTableHeader(line: string): boolean {
  return line.trim().startsWith('[');
}

// unwrapModuleConfig strips the module's own [modules_config.<id>] header so the
// form shows flat keys. Documents shaped any other way pass through untouched.
export function unwrapModuleConfig(configToml: string, moduleId: string): string {
  const lines = configToml.split('\n');
  const tables = lines.filter(isTableHeader);
  if (tables.length !== 1 || tables[0].trim() !== header(moduleId)) return configToml;
  return lines
    .filter((l) => !isTableHeader(l))
    .join('\n')
    .replace(/^\n+/, '');
}

// wrapModuleConfig is the inverse. Text that already declares a
// [modules_config.…] table is sent as typed, so an operator can hand-write the
// full document if a module needs more than one table.
export function wrapModuleConfig(flat: string, moduleId: string): string {
  if (flat.trim() === '') return '';
  if (flat.includes('[modules_config.')) return flat;
  return `${header(moduleId)}\n${flat.replace(/^\n+/, '')}`;
}

/** One `key = value` line, or a line the form does not model (comment, blank). */
type Entry = { key: string; value: string } | { raw: string };

const KEY_VALUE = /^\s*([A-Za-z_][A-Za-z0-9_-]*)\s*=\s*(.*?)\s*$/;

function splitEntries(flat: string): Entry[] {
  return flat
    .split('\n')
    .filter((l, i, all) => l.trim() !== '' || i < all.length - 1)
    .map((line) => {
      const m = KEY_VALUE.exec(line);
      return m ? { key: m[1], value: unquote(m[2]) } : { raw: line };
    });
}

function unquote(v: string): string {
  if (v.length >= 2 && ((v.startsWith('"') && v.endsWith('"')) || (v.startsWith("'") && v.endsWith("'")))) {
    return v.slice(1, -1).replace(/\\"/g, '"').replace(/\\\\/g, '\\');
  }
  return v;
}

// quote renders a value the way TOML expects for the field's declared type.
// Unknown types are treated as strings, which is the safe direction: a quoted
// number still parses, an unquoted string does not.
function quote(value: string, type: string): string {
  if (type === 'number' && value.trim() !== '' && Number.isFinite(Number(value))) return value.trim();
  if (type === 'bool' && (value === 'true' || value === 'false')) return value;
  return `"${value.replace(/\\/g, '\\\\').replace(/"/g, '\\"')}"`;
}

/** parseFlatConfig reads a flat config body into key → display value. */
export function parseFlatConfig(flat: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const e of splitEntries(flat)) {
    if ('key' in e) out[e.key] = e.value;
  }
  return out;
}

/**
 * buildFlatConfig writes the form's values back as a flat config body: declared
 * fields in manifest order, then anything the original document had that the
 * schema does not model, so hand-written extras and comments survive a form save.
 * A field left empty is omitted rather than written blank, which is what lets the
 * module keep applying its own default.
 */
export function buildFlatConfig(
  values: Record<string, string>,
  fields: Array<{ key: string; type: string }>,
  originalFlat = '',
): string {
  const declared = new Set(fields.map((f) => f.key));
  const lines = fields
    .filter((f) => (values[f.key] ?? '').trim() !== '')
    .map((f) => `${f.key} = ${quote(values[f.key], f.type)}`);
  const kept = splitEntries(originalFlat)
    .filter((e) => ('key' in e ? !declared.has(e.key) : e.raw.trim() !== ''))
    .map((e) => ('key' in e ? `${e.key} = ${quote(e.value, 'string')}` : e.raw));
  const all = [...lines, ...kept];
  return all.length === 0 ? '' : all.join('\n') + '\n';
}
