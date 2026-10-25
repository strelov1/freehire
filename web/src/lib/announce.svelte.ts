// One place to tell a screen-reader user what just happened.
//
// A visible "Saved" or an inline error is silent to assistive technology unless it sits
// inside a live region that already existed when the text changed — a region mounted
// together with its message is usually not read at all. So the regions live once, in the
// root layout (`Announcer.svelte`), and every component speaks through `announce()`
// instead of keeping its own.
//
// A module-level $state singleton, safe under SSR for the same reason `assistantDock`
// is: it defaults to nothing and is only ever written from browser interactions.

export type Politeness = 'polite' | 'assertive';

/**
 * How long a region stays empty before the new message lands. A screen reader announces a
 * MUTATION of a live region, so the same message twice in a row ("Saved", "Saved") has to
 * pass through an empty value the reader can observe — set in one tick, the two writes
 * collapse into no change at all.
 */
export const ANNOUNCE_DELAY_MS = 100;

const regions = $state<Record<Politeness, string>>({ polite: '', assertive: '' });
// Messages waiting out the delay. A second call inside it joins the first rather than
// replacing it: "Saved" followed at once by "Moved to Interview" must say both.
const pending: Record<Politeness, string[]> = { polite: [], assertive: [] };
const timers: Record<Politeness, ReturnType<typeof setTimeout> | undefined> = {
  polite: undefined,
  assertive: undefined,
};

/** The text a live region currently holds. Read by `Announcer.svelte`. */
export function announcement(politeness: Politeness): string {
  return regions[politeness];
}

// One sentence per message, so two of them read as two rather than run together.
const asSentence = (m: string) => (/[.!?…]$/.test(m) ? m : `${m}.`);

/**
 * Speak `message` to assistive technology. Errors are `assertive` — they interrupt; every
 * confirmation is `polite`. The visible text stays where it is: this only adds a voice.
 */
export function announce(message: string, politeness: Politeness = 'polite'): void {
  pending[politeness].push(message);
  if (timers[politeness] !== undefined) return;
  regions[politeness] = '';
  timers[politeness] = setTimeout(() => {
    const queued = pending[politeness];
    pending[politeness] = [];
    timers[politeness] = undefined;
    // `message` is the first queued: only the call that found no timer scheduled one.
    regions[politeness] = queued.length > 1 ? queued.map(asSentence).join(' ') : message;
  }, ANNOUNCE_DELAY_MS);
}
