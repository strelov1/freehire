import type { SearchPlaceholder } from '#lib/typedPlaceholder.svelte.js';
import type { FacetCounts } from '#lib/types.js';

/**
 * The header's search box — the ONE of them, on every page.
 *
 * There were two: this, which filtered the list under it, and a launcher on every
 * other page, which navigated to the feed. They shared the debounce, the stale-answer
 * guard, the arrow keys, the hotkeys, the dismissal and the row rendering, in two
 * copies, and differed in exactly one thing: what a pick DOES. So
 * that one thing is a target now (HeaderSearch's `target`), and everything else is
 * written once.
 *
 * Typing does NOT run the search. What you type is a draft; Enter or choosing a row
 * commits it. The box used to push every keystroke into the store, so the feed
 * refetched while the visitor was still composing — and the half-typed word it
 * searched for was rarely the one they meant.
 *
 * `size` and `autofocus` are presentation. The homepage renders this same box, at
 * hero size, as the whole of its content (see HomeLandingView) — it registers no
 * list, so it gets the browse target (LauncherTarget) and every pick becomes a link to the
 * feed. A hero-sized second copy of this component is how the two would drift.
 */
export interface HeaderSearchProps {
  /** What an empty box shows.
   *
   *  A plain string is static. `{ prefix, roles }` types the roles one after another
   *  into the tail of a sentence whose `prefix` never moves — see lib/placeholderRoles.ts,
   *  which owns both the words and the stepping.
   *
   *  One prop rather than a static one plus an animated override, because an override
   *  would always win and leave the static string dead at every animating call site —
   *  the reader would have two candidates and no way to tell which one ships. Either
   *  way this is never the field's accessible name; that is `label`. */
  placeholder: SearchPlaceholder;
  /** The field's accessible name. Static, and required of every caller rather than
   *  defaulting to `placeholder`: the two were one prop until the placeholder learned
   *  to rotate, at which point a screen reader announced the input as whatever example
   *  happened to be on screen. A default would reinstate that at the next call site. */
  label: string;
  size?: 'header' | 'hero';
  /** Focus the box on mount — desktop only. A page whose whole content is this box
   *  should put the caret in it; on a phone the same call raises the keyboard over
   *  the page before the visitor has decided to search, so the width check below is
   *  the feature rather than a fallback. */
  autofocus?: boolean;
  /** A filter modal the HOST owns, for a page with no list of its own — the
   *  homepage, where picking filters composes a search rather than narrowing a list.
   *  Where a list IS registered its own modal wins, so this is never a second way to
   *  open the same thing. */
  onOpenFilters?: () => void;
  /** The category distribution behind the empty box, when the page has already
   *  measured it. Off a list page this is otherwise fetched here on first focus;
   *  a caller that server-rendered the same numbers passes them instead of making
   *  the browser ask for them again. */
  counts?: FacetCounts | null;
}
