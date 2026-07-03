import { cn } from '$lib/utils';

// The shared three-state look of a selectable facet pill: idle (secondary fill),
// selected-include (primary fill), selected-exclude (muted destructive,
// struck through). Callers add their own size classes via `extra`.
export function pillClass(active: boolean, exclude: boolean, extra = ''): string {
  return cn(
    'rounded-full border font-medium transition-colors active:translate-y-px',
    !active && 'border-transparent bg-secondary text-secondary-foreground hover:bg-accent',
    active && !exclude && 'border-transparent bg-primary text-primary-foreground',
    active && exclude && 'border-destructive/30 bg-destructive/15 text-destructive line-through',
    extra,
  );
}

// Tooltip hint for a three-state facet pill, naming the state its next click
// produces. Excludable facets cycle off → include → exclude → off; non-excludable
// ones just toggle off ↔ include, so they never advertise an exclude step.
export function pillTitle(included: boolean, excluded: boolean, excludable: boolean): string | undefined {
  if (excluded) return 'Click to remove';
  if (included) return excludable ? 'Click to exclude' : 'Click to remove';
  return excludable ? 'Click to include' : undefined;
}
