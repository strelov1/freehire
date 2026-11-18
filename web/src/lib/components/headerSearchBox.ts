// The header search box's frame: how it is drawn, how a press on it reaches the field, and
// what its keys do.

import { cn } from '#lib/ui/index.js';

/** Tailwind's `sm`. Two things in HeaderSearch ask it — whether to place the caret on mount,
 *  and how the box's panel is drawn — and they must get one answer, since the markup they
 *  are reasoning about switches on the same breakpoint. */
export const WIDE_QUERY = '(min-width: 640px)';

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

/**
 * What a key in the box does: `enter` searches (or activates the highlighted row), `close`
 * puts the dropdown away, `down`/`up` move the highlight, null leaves the key alone.
 *
 * Mid-composition (CJK/IME) Enter CONFIRMS a candidate and the arrows move through them;
 * the browser must keep those. `oninput` has already fired by then, so the draft holds
 * pre-conversion text — searching it would search a half-typed word.
 *
 * Enter is handled whether or not the dropdown is open — it is the only way typing reaches
 * the list now, so it cannot sit behind the dropdown's guard. Every other key belongs to the
 * dropdown, so with it closed nothing else is claimed — which keeps the input behaving as it
 * does where no suggestions exist. Escape keeps the typed text; only the dropdown closes.
 */
export function boxKey(e: KeyboardEvent, open: boolean): 'enter' | 'close' | 'down' | 'up' | null {
  if (e.isComposing) return null;
  if (e.key === 'Enter') return 'enter';
  if (!open) return null;
  if (e.key === 'Escape') return 'close';
  if (e.key === 'ArrowDown') return 'down';
  if (e.key === 'ArrowUp') return 'up';
  return null;
}

/** The highlight after an arrow, wrapping at either end of `count` rows. */
export function stepIndex(index: number, count: number, dir: 'down' | 'up'): number {
  if (dir === 'down') return index < count - 1 ? index + 1 : 0;
  return index > 0 ? index - 1 : count - 1;
}

/** Same global hotkeys as the launcher: Cmd/Ctrl+K always, `/` unless typing. */
export function isFocusHotkey(e: KeyboardEvent): boolean {
  if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') return true;
  if (e.key === '/' && !e.metaKey && !e.ctrlKey) {
    const tag = (document.activeElement as HTMLElement | null)?.tagName;
    return tag !== 'INPUT' && tag !== 'TEXTAREA';
  }
  return false;
}
