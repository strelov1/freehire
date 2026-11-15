import type { SearchPlaceholder } from '#lib/typedPlaceholder.svelte.js';
import type { FacetCounts } from '#lib/types.js';

/** HeaderSearch's props; what the component does with them is told beside its `$props()`. */
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
