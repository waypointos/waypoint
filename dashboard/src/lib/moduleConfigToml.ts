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
