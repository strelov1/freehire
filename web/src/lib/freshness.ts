import type { Reality } from './generated/contracts';

/** A badge on the posting's title row: how recently it went up, and whether anyone
 *  has beaten the reader to it. Both read as encouragement, so both are held to a
 *  higher bar than "the number looks small" — see `freshnessBadges`. */
export interface FreshnessBadge {
  /** Chip text. */
  label: string;
  /** The fact behind it, on hover — the claim's own limits, not a restatement. */
  tooltip: string;
}

/** A posting is "new" for a week. Longer and the word stops meaning anything on a
 *  board where the median role is open for a month. */
const NEW_DAYS = 7;
/** "Early applicant" is a claim about a window, not about a count: three days is
 *  as far as we are willing to make it. */
const EARLY_DAYS = 3;
/** …and only while the field is genuinely still empty. This counts the people who
 *  told US they applied, which is a floor on the real number and never a measure of
 *  it, so the threshold is small enough that the claim survives being wrong by an
 *  order of magnitude. */
const EARLY_APPLIES = 3;

/** daysSince returns whole days between `iso` and now, or null when the date is
 *  missing or unparseable. Clamped at zero: a source clock running a few hours ahead
 *  is ordinary and must not read as a posting from the future. */
export function daysSince(iso?: string | null): number | null {
  if (!iso) return null;
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return null;
  return Math.max(0, Math.floor((Date.now() - t) / 86_400_000));
}

function postedPhrase(days: number): string {
  if (days === 0) return 'posted today';
  if (days === 1) return 'posted yesterday';
  return `posted ${days} days ago`;
}

/** The early-applicant tooltip. It names who was counted, because the count is our own
 *  applied-marks and not the employer's inbox. */
function appliedPhrase(count: number, posted: string): string {
  if (count === 0) return `Nobody has told us they applied to this job yet (${posted}).`;
  const who = count === 1 ? 'person has' : 'people have';
  return `${count} ${who} told us they applied to this job (${posted}).`;
}

/** freshnessBadges renders the "New" / "Be an early applicant" pair for a posting.
 *
 *  The posting date alone does not earn either badge. `reality` is the system's own
 *  reading of how long this job has actually been open, and it knows the case this
 *  page would otherwise walk into: a role open for eight months whose source rewrites
 *  its posting date every crawl (`fake_freshness`). Trusting `posted_at` there would
 *  print "New" on the oldest job in the catalogue — so a posting the signal has
 *  classified as anything but fresh gets no badge at all, and neither does one whose
 *  date the signal distrusts. When the signal is absent (a projection that carries no
 *  counts) the date stands on its own; that is the ordinary case for a job we have
 *  only just met.
 *
 *  `appliedCount` is the number of signed-in users who marked this job applied HERE.
 *  It cannot see the employer's own inbox, so "be an early applicant" is offered as
 *  an invitation and the tooltip says exactly what was counted. */
export function freshnessBadges(
  postedAt?: string | null,
  reality?: Reality | null,
  appliedCount = 0,
): FreshnessBadge[] {
  if (reality && (reality.class !== 'fresh' || reality.fake_freshness)) return [];
  const days = daysSince(postedAt);
  if (days === null || days > NEW_DAYS) return [];

  const posted = postedPhrase(days);
  const badges: FreshnessBadge[] = [{ label: 'New', tooltip: posted }];

  if (days <= EARLY_DAYS && appliedCount <= EARLY_APPLIES) {
    badges.push({ label: 'Be an early applicant', tooltip: appliedPhrase(appliedCount, posted) });
  }
  return badges;
}
