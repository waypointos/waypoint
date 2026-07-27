// dashboard/src/state/initials.ts
//
// Derives the 1-2 character "avatar pill" string from whatever identity
// signal the dashboard has at hand. In proxy mode the signed-in user's
// email is the source; in local mode the only identity available is the
// rover id (no signed-in user — the bootstrap token is shared).
//
// We never invent placeholder text ("ME", "DEV", etc.). If neither source
// is present, return "" — the Avatar component renders an empty pill.
import type { Me } from './mode';

export function displayInitials(me: Me | null | undefined): string {
  if (!me) return '';
  if (me.email) return initialsFromCompound(localPart(me.email), '.');
  if (me.roverId) return initialsFromCompound(me.roverId, '-');
  return '';
}

function localPart(email: string): string {
  const at = email.indexOf('@');
  return at >= 0 ? email.slice(0, at) : email;
}

// Splits `s` on `sep` and returns up to 2 leading letters: one from each of
// the first two parts when there are multiple parts, or the first two
// characters when there's just one.
function initialsFromCompound(s: string, sep: string): string {
  const parts = s.split(sep).filter(Boolean);
  if (parts.length >= 2) {
    return (parts[0][0] + parts[1][0]).toUpperCase();
  }
  if (parts.length === 1) {
    return parts[0].slice(0, 2).toUpperCase();
  }
  return '';
}
