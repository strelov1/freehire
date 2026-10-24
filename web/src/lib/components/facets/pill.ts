import { cn } from '#lib/ui/index.js';

// The shared three-state look of a selectable facet pill: idle (secondary fill),
// selected-include (brand fill), selected-exclude (muted destructive,
// struck through). Callers add their own size classes via `extra`.
//
// This is the FILTER pill, and not the only pill in the app: stating something about
// yourself is two-state and bordered, which is profile/SeniorityPills.svelte. Two looks
// on purpose — a facet you are narrowing by and an answer you are giving read
// differently — so reach for that one before adding a fourth state here.
export function pillClass(active: boolean, exclude: boolean, extra = ''): string {
  return cn(
    'rounded-full border font-medium transition-colors active:translate-y-px',
    !active && 'border-transparent bg-secondary text-secondary-foreground hover:bg-accent',
    active && !exclude && 'border-transparent bg-brand text-brand-foreground',
    active && exclude && 'border-destructive/30 bg-destructive/15 text-destructive-strong line-through',
    extra,
  );
}

// The option count beside a pill's label. It fades on the idle fill only: a selected
// pill's brand or destructive fill leaves no contrast to spare (60% measured ~2.9:1,
// under WCAG AA's 4.5:1), so there the lighter weight alone keeps it secondary.
export function pillCountClass(active: boolean): string {
  return cn('ml-1 font-normal tabular-nums', !active && 'opacity-60');
}

// Tooltip hint for a three-state facet pill, naming the state its next click
// produces. Excludable facets cycle off → include → exclude → off; non-excludable
// ones just toggle off ↔ include, so they never advertise an exclude step.
export function pillTitle(included: boolean, excluded: boolean, excludable: boolean): string | undefined {
  if (excluded) return 'Click to remove';
  if (included) return excludable ? 'Click to exclude' : 'Click to remove';
  return excludable ? 'Click to include' : undefined;
}

// What a three-state pill tells assistive tech, since its look (fill, strikethrough)
// and its hover title reach neither a screen reader nor a touch user. One pattern for
// every filter pill:
//
//   off      → aria-pressed="false"
//   include  → aria-pressed="true"
//   exclude  → aria-pressed="true", plus a visually hidden ", excluded" after the label
//
// Pressed means "this value is filtering", whichever way; the hidden word says which
// way. Not aria-pressed="mixed": that reads as "partially pressed", which is not what
// an exclusion is, and screen readers announce it inconsistently. PillMeta.svelte
// renders the hidden word together with the count, so a pill site needs only this
// attribute and that component.
export function pillPressed(included: boolean, excluded: boolean): boolean {
  return included || excluded;
}
