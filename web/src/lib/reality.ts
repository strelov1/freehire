import type { Reality } from './generated/contracts';

/** A rendered job-reality badge: a tone and a compact label, plus the full fact
 *  string that justifies it. `null` means show no badge (a fresh or unclassified
 *  job). We state facts ("Open 240 days · reposted 6×"), never a bare accusation. */
export interface RealityBadge {
  tone: 'warn' | 'muted';
  label: string;
  facts: string;
}

/** facts assembles the observable evidence behind a non-fresh classification. */
function facts(r: Reality): string {
  const parts = [`Open ${r.age_days} days`];
  if (r.repost_count > 1) parts.push(`reposted ${r.repost_count}×`);
  if (r.mass_posting_count > 1) parts.push(`${r.mass_posting_count} open copies`);
  if (r.fake_freshness) parts.push('posting date refreshed');
  return parts.join(' · ');
}

/** realityBadge maps the served reality signal to a badge, or null when there is
 *  nothing to show (fresh or missing). */
export function realityBadge(reality?: Reality | null): RealityBadge | null {
  if (!reality || reality.class === 'fresh') return null;
  if (reality.class === 'likely-evergreen') {
    return { tone: 'warn', label: 'Likely evergreen', facts: facts(reality) };
  }
  // stale
  return { tone: 'muted', label: `Open ${reality.age_days}d`, facts: facts(reality) };
}
