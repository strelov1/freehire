// Shared bounds/presets for the salary and freshness sliders, used by both the
// legacy FiltersPanel sidebar and the new filter modal so the two never drift.

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

/** Label for the current freshness value; a non-preset value reads as "Any". */
export function freshnessLabel(days: number | null): string {
  return FRESHNESS_PRESETS.find((p) => p.days === days)?.label ?? 'Any';
}
