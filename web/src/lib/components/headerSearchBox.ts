// The header search box's frame: how it is drawn, and how a press on it reaches the field.

import { cn } from '#lib/ui/index.js';

export function searchBoxClass(hero: boolean, autoFocused: boolean): string {
  return cn(
    'flex items-center border border-border bg-background',
    !autoFocused && 'focus-within:ring-2 focus-within:ring-ring',
    hero
      ? // Taller, rounder and lifted off the page: at the centre of an otherwise
        // empty screen this is the only interactive thing, so it carries the weight
        // the header version borrows from the bar around it.
        'h-14 gap-3 rounded-2xl px-4 text-base shadow-lg shadow-foreground/5 sm:h-16 sm:px-5'
      : // 48px inside the bar's own 56px. The header height is `h-14` in eight other
        // places (`top-14`, `top-20`, `PINNED_HEADER_TOP`, the full-bleed
        // `calc(100dvh-3.5rem)`), so it is the field that grows into the bar rather
        // than the bar that grows.
        'h-12 gap-2 rounded-md px-3 text-sm',
  );
}

// The whole box takes the caret, not just the field inside it. Probed at 390px: the
// field is 175px of a 278px box, and the other 103 — the rule, the magnifier, the
// `/` hint, the padding — landed on nothing at all. That reads as a search box that
// ignored the first tap and answered the second, better-aimed one.
//
// Only what is NOT itself a control: the scope prefix, the clear button and the
// All-filters trigger keep their own taps. `preventDefault` because the default for
// a press on a non-focusable box is to take focus AWAY from the field.
export function focusFieldFromBox(e: PointerEvent, input: HTMLInputElement | null): void {
  if ((e.target as HTMLElement | null)?.closest('button, input, a')) return;
  e.preventDefault();
  input?.focus();
}
