// dashboard/src/ui/tokens.ts
//
// Same tokens as tokens.css, exposed as TS for code that needs them
// (e.g., uPlot themes, inline SVG fills, JS-driven animations).
// If you change one, change the other.

export const color = {
  bg:          '#0A0A0B',
  chrome:      '#0F0F11',
  surface:     '#131316',
  surface2:    '#1B1B1F',
  raise:       '#232328',
  border:      '#26262C',
  borderSoft:  '#18181C',

  fg:    'rgba(255, 255, 255, 0.96)',
  fg2:   '#C2C2C8',
  fg3:   '#828289',
  fg4:   '#565660',

  accent:       '#0A84FF',
  accentHover:  '#0070E0',
  accentTint:   'rgba(10, 132, 255, 0.14)',
  caution:      '#FFCC00',
  cautionTint:  'rgba(255, 204, 0, 0.10)',
  ok:           '#30D158',
  fault:        '#FF453A',
} as const;

export const space = {
  xs: 4, sm: 8, md: 12, lg: 16, xl: 24, xxl: 40,
} as const;

export const font = {
  sans:    `-apple-system, BlinkMacSystemFont, 'Inter', 'Segoe UI', sans-serif`,
  display: `-apple-system, 'SF Pro Display', 'Helvetica Neue', system-ui, sans-serif`,
  mono:    `'JetBrains Mono', 'SF Mono', ui-monospace, monospace`,
} as const;

export const motion = {
  fast: '80ms ease-out',
  base: '120ms ease-out',
} as const;
