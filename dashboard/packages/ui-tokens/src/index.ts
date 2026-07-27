// Waypoint design tokens, exported as TypeScript constants for chart
// libraries and other consumers that need inline style values. The
// authoritative source is dashboard/src/ui/tokens.css; this file is
// regenerated when tokens change.

export const colors = {
  bg:          '#0A0A0B',
  chrome:      '#0F0F11',
  surface:     '#131316',
  surface2:    '#1B1B1F',
  raise:       '#232328',
  border:      '#26262C',
  borderSoft:  '#18181C',
  fg:          'rgba(255, 255, 255, 0.96)',
  fg2:         '#C2C2C8',
  fg3:         '#828289',
  fg4:         '#565660',
  accent:      '#0A84FF',
  accentTint:  'rgba(10, 132, 255, 0.14)',
  caution:     '#FFCC00',
  cautionTint: 'rgba(255, 204, 0, 0.10)',
  ok:          '#30D158',
  fault:       '#FF453A',
} as const;

export const space = {
  xs: '4px', sm: '8px', md: '12px', lg: '16px', xl: '24px', xxl: '40px',
} as const;

export const fonts = {
  sans:    `-apple-system, BlinkMacSystemFont, 'Inter', 'Segoe UI', sans-serif`,
  display: `-apple-system, 'SF Pro Display', 'Helvetica Neue', system-ui, sans-serif`,
  mono:    `'JetBrains Mono', 'SF Mono', ui-monospace, monospace`,
} as const;

export type WaypointTokens = {
  colors: typeof colors;
  space:  typeof space;
  fonts:  typeof fonts;
};

export const tokens: WaypointTokens = { colors, space, fonts };
