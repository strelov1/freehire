import type { Reality } from './generated/contracts';

/** A rendered job-reality badge: a tone and a compact chip label, plus two fuller
 *  strings. `tooltip` is the complete justification (age + evidence) shown on hover;
 *  `evidence` is the same minus the age the label already carries, so the detail view
 *  can show complementary facts without repeating "Open N days". `null` means show no
 *  badge (a fresh or unclassified job). We state facts, never a bare accusation. */
export interface RealityBadge {
  tone: 'warn' | 'muted';
  label: string;
  tooltip: string;
  evidence: string;
}

/** evidenceParts is the observable evidence behind a non-fresh classification,
 *  EXCLUDING the age — the badge label already carries it, so restating it here would
 *  read as "Open 21 days" twice beside an "Open 21d" chip. */
function evidenceParts(r: Reality): string[] {
  const parts: string[] = [];
  if (r.repost_count > 1) parts.push(`reposted ${r.repost_count}×`);
  if (r.mass_posting_count > 1) parts.push(`${r.mass_posting_count} open copies`);
  if (r.fake_freshness) parts.push('posting date refreshed');
  return parts;
}

/** realityBadge maps the served reality signal to a badge, or null when there is
 *  nothing to show (fresh or missing). */
export function realityBadge(reality?: Reality | null): RealityBadge | null {
  if (!reality || reality.class === 'fresh') return null;
  const parts = evidenceParts(reality);
  const evidence = parts.join(' · ');
  const tooltip = [`Open ${reality.age_days} days`, ...parts].join(' · ');
  if (reality.class === 'likely-evergreen') {
    return { tone: 'warn', label: 'Likely evergreen', tooltip, evidence };
  }
  // stale
  return { tone: 'muted', label: `Open ${reality.age_days}d`, tooltip, evidence };
}
