/** The profile's specialization cap, mirroring the server's userprofile.MaxSpecializations
 *  (and the cardinality CHECK behind it, migration 0135).
 *
 *  One copy, because there are three surfaces that pick specializations — the profile form,
 *  the profile's Roles card, and the onboarding wizard — and the wizard is the one that
 *  shipped without a cap at all: it unioned whatever a CV or LinkedIn import resolved into
 *  the staged set, sent the lot, and swallowed the server's 400. A number written out four
 *  times is a number that goes missing in one of them.
 */
export const MAX_SPECIALIZATIONS = 10;

/** Folds `incoming` into `current` up to the cap, keeping the order the user already sees
 *  and reporting what the cap left out — every caller has something to say about the
 *  remainder ("Reached the limit — nothing more added"), so dropping it silently is the one
 *  behaviour none of them want.
 *
 *  Returns `current` itself when nothing was added and nothing was dropped, so a merge that
 *  changes nothing does not churn the caller's reactive state. */
export function capSpecializations(current: string[], incoming: string[]): { kept: string[]; dropped: number } {
  const kept = [...current];
  let dropped = 0;
  for (const value of incoming) {
    if (kept.includes(value)) continue;
    if (kept.length < MAX_SPECIALIZATIONS) kept.push(value);
    else dropped++;
  }
  if (dropped === 0 && kept.length === current.length) return { kept: current, dropped: 0 };
  return { kept, dropped };
}
