import type { Reality } from './generated/contracts';
import { timeAgo } from './utils';

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

/** postingContrast returns a "posting dated N ago" note when the source's posting date
 *  reads meaningfully fresher than the job's true age — the refreshed-date story the
 *  age label alone hides ("open 21 days, but posting dated 2 days ago"). '' when there
 *  is no posting date, it is unparseable, or the gap is too small to be worth stating. */
export function postingContrast(reality: Reality, postedAt?: string | null): string {
  if (!postedAt) return '';
  const d = new Date(postedAt);
  if (Number.isNaN(d.getTime())) return '';
  const postedDays = Math.floor((Date.now() - d.getTime()) / 86_400_000);
  // The badge only shows for non-fresh jobs (age > 14d); a posting reading a clear
  // week fresher than that true age is the contrast worth surfacing.
  if (reality.age_days - postedDays < 7) return '';
  const ago = timeAgo(postedAt);
  return ago ? `posting dated ${ago}` : '';
}
