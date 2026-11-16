import type { computeClientMatch } from '#lib/jobMatch.js';
import type { Snippet } from 'svelte';
import type { Job, JobCard } from '#lib/types.js';

/**
 * Single source of truth for how a job appears in any list (jobs list and
 * company detail). The whole card is a link to the job detail.
 *
 * `dimViewed` dims the card when the signed-in user has already viewed this
 * job, so the browse list shows what's been seen. The My Jobs surfaces (where
 * every card is viewed by definition) pass `dimViewed={false}` to opt out.
 * `newTab` opens the job in a new browser tab (used when the card is rendered
 * inside the assistant chat, so the conversation stays open). Both default off so the
 * jobs list / company pages are unchanged.
 *
 * `compact` is the narrow-chat-column card, and it decides more than density. As well
 * as the smaller padding and title, the one-line title and the dropped blurb, it
 * selects the card's whole control inventory: there is no action row, so Save stays
 * the corner glyph it always was and `onHide` has nowhere to render. The two concerns
 * are fused on purpose — the column is ~360px wide, and every control that earns a
 * word on a full card has to give it back here. Splitting them would be a second flag
 * whose only honest value is the same as this one's.
 *
 * `footer` is an optional actions row rendered inside the card, below the link
 * content (a sibling of the <a>, never nested in it — so its interactive controls
 * don't fight the card's navigation). The hidden list passes an un-hide control here,
 * and gets it as a second row under the card's own Save row.
 *
 * `onHide` is the feed's hook for the "hide this job" gesture: when set on a NON-compact
 * card (only the browse feed passes it), the action row gains a hover-revealed hide
 * control, and after a successful dismiss it calls back with the slug so the feed can
 * surface an undo affordance. Surfaces that reuse JobRow without it (saved/hidden
 * lists, tracking board, assistant chat) get no hide control — leaving it out scopes
 * the gesture to the feed.
 */
export interface JobRowProps {
  // Either the catalogue's full posting or the tracking listing's card. The row draws the
  // same thing from both: the card carries a server-cut blurb where the posting carries the
  // whole description, and the two facet fields a full Job keeps inside `enrichment` arrive
  // flat. Everything the card omits is optional here already.
  job: Job | JobCard;
  clientMatch?: ReturnType<typeof computeClientMatch>;
  dimViewed?: boolean;
  newTab?: boolean;
  compact?: boolean;
  // On the employer's own page every card would repeat its logo, name and backers.
  hideCompany?: boolean;
  // A row in a divided list (the feed draws the frame and the dividers), not a framed card.
  flat?: boolean;
  footer?: Snippet;
  onHide?: (slug: string) => void;
  /** Allows a saved list to serialize removal with pagination. */
  unsave?: (slug: string) => Promise<unknown>;
}
