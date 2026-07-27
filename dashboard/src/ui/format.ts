// dashboard/src/ui/format.ts
//
// Fixed-width formatting for live mono readouts. Padding with no-break spaces
// (digit-width in mono fonts) keeps a value from shifting layout when its
// digit count changes (e.g. +9.8 -> +10.2).
export const NBSP = ' ';

export function padFixed(s: string, width: number): string {
  return s.padStart(width, NBSP);
}

/** Explicit-sign decimal: +0.42 / -1.30. */
export function signed(v: number, digits = 2): string {
  return (v >= 0 ? '+' : '') + v.toFixed(digits);
}

export function signedFixed(v: number, digits = 2, width = 5): string {
  return padFixed(signed(v, digits), width);
}
