// Deterministic sender avatar for the inbox: initials + a stable colour picked
// from a curated palette (seeded by the address), so the message list gets a
// visual anchor without fetching external logos. Pure + unit-tested.

// Muted, saturated-enough hues that read as white-on-colour on both the light
// and dark card backgrounds. Deliberately not the brand olive — the avatar is a
// per-sender accent, not a CTA.
const PALETTE = [
  '#b45309', // amber
  '#0f766e', // teal
  '#7c3aed', // violet
  '#be123c', // rose
  '#1d4ed8', // blue
  '#4d7c0f', // olive-lime
  '#c2410c', // orange
  '#0369a1', // sky
  '#9333ea', // purple
  '#0d9488', // emerald-teal
];

/** One or two uppercase initials from the sender's display name, falling back to
 *  the address local-part, then '?'. "n8n Hiring Team" → "NH"; "g-mate" → "G". */
export function avatarInitials(name: string, addr: string): string {
  const src = (name || addr || '').trim();
  if (!src) return '?';
  const words = src.split(/\s+/).filter(Boolean);
  if (words.length >= 2) {
    const a = words[0]?.replace(/[^a-zA-Z0-9]/g, '')[0];
    const b = words[1]?.replace(/[^a-zA-Z0-9]/g, '')[0];
    if (a && b) return (a + b).toUpperCase();
  }
  const alnum = src.replace(/[^a-zA-Z0-9]/g, '');
  return (alnum[0] ?? '?').toUpperCase();
}

/** A stable palette colour for a sender, seeded by a string (address preferred). */
export function avatarColor(seed: string): string {
  let h = 0;
  for (let i = 0; i < seed.length; i++) {
    h = (h * 31 + seed.charCodeAt(i)) >>> 0;
  }
  return PALETTE[h % PALETTE.length] ?? PALETTE[0]!;
}
