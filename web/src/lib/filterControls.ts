// Shared bounds/presets for the salary and freshness sliders, used by the filter
// modal (and the analytics facet breakdowns) so the controls never drift.

export const SALARY_MAX = 300000;
export const SALARY_STEP = 5000;

/** Freshness presets, oldest→newest left→right with "Any" as the rightmost stop. */
export const FRESHNESS_PRESETS: { days: number | null; label: string }[] = [
  { days: 1, label: 'Today' },
  { days: 3, label: '3 days' },
  { days: 7, label: '1 week' },
  { days: 14, label: '2 weeks' },
  { days: 30, label: '1 month' },
  { days: 90, label: '3 months' },
  { days: null, label: 'Any' },
];

/** Label for the current freshness value. Like experienceLabel, this does NOT fall
 *  back to "Any" for an off-preset value: a shared link, a hand-edited URL or an
 *  AI-built filter can carry any day count, and reporting a live bound as "Any" would
 *  tell the user the freshness filter is off while it quietly hides every posting
 *  older than that bound. */
export function freshnessLabel(days: number | null): string {
  const preset = FRESHNESS_PRESETS.find((p) => p.days === days);
  if (preset) return preset.label;
  if (days == null) return 'Any';
  return `Last ${days} ${days === 1 ? 'day' : 'days'}`;
}

/** The freshness stops a SELECT offers, given the bound currently in force.
 *
 *  The presets, plus the current bound itself when it is not one of them. A select can
 *  only show a value it has an option for: a bound arriving from a shared link, a
 *  hand-edited URL or the AI filter dialog (which writes whatever day count it read)
 *  matches nothing, leaves `selectedIndex = -1`, and renders the control BLANK while it
 *  quietly hides every older posting — the same lie freshnessLabel exists to prevent,
 *  told by the control instead of by the label.
 *
 *  The off-preset stop is inserted in day order so the list still reads oldest→newest,
 *  and it disappears the moment the user picks a real preset. The modal's slider solves
 *  the same input differently (it clamps the handle and labels it honestly), because a
 *  slider has no option list to be absent from. */
export function freshnessOptions(days: number | null): { days: number | null; label: string }[] {
  if (days == null || FRESHNESS_PRESETS.some((p) => p.days === days)) return FRESHNESS_PRESETS;
  const at = FRESHNESS_PRESETS.findIndex((p) => p.days != null && p.days > days);
  const extra = { days, label: freshnessLabel(days) };
  return at < 0
    ? [...FRESHNESS_PRESETS.slice(0, -1), extra, ...FRESHNESS_PRESETS.slice(-1)]
    : [...FRESHNESS_PRESETS.slice(0, at), extra, ...FRESHNESS_PRESETS.slice(at)];
}

/** Experience-ceiling presets, least→most with "Any" as the rightmost stop. Each
 *  stop is an upper bound on the years a posting asks for, so the leftmost is a
 *  real `0` — the postings stating no prior experience is required — and only the
 *  rightmost is "no bound".
 *
 *  The spacing is deliberately non-linear because the catalogue is: measured on
 *  production, postings asking 5–7 years outnumber those asking 8–9 by more than
 *  five to one, so evenly spaced stops would spend most of the slider's travel on
 *  a thin tail. */
export const EXPERIENCE_PRESETS: { years: number | null; label: string }[] = [
  { years: 0, label: 'No experience' },
  { years: 1, label: 'Up to 1 year' },
  { years: 2, label: 'Up to 2 years' },
  { years: 3, label: 'Up to 3 years' },
  { years: 5, label: 'Up to 5 years' },
  { years: 8, label: 'Up to 8 years' },
  { years: 10, label: 'Up to 10 years' },
  { years: null, label: 'Any' },
];

/** Label for the current experience ceiling. Unlike freshnessLabel this does NOT
 *  fall back to "Any" for an off-preset value: a hand-edited or shared URL can
 *  carry any year count, and reporting a live bound as "Any" would tell the user
 *  the filter is off while it is quietly narrowing their results. */
export function experienceLabel(years: number | null): string {
  const preset = EXPERIENCE_PRESETS.find((p) => p.years === years);
  if (preset) return preset.label;
  if (years == null) return 'Any';
  return `Up to ${years} ${years === 1 ? 'year' : 'years'}`;
}
