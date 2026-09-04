// What happens to a link pasted into the search box.
//
// Two doors, in order, because they cost different things. `/jobs/find` is a public
// read: an indexed lookup of the URL against the catalog, no page fetched, nothing
// written, and open to a signed-out visitor. `/jobs/resolve` is the intake: it goes out
// to the page, may import the vacancy, and records the board behind it against the
// caller — so it needs an account, and it is the slow one.
//
// The cheap public door therefore runs FIRST, for everybody. A visitor whose link we
// already carry gets their vacancy without being asked to sign in, which is the common
// case and the whole point of recognising the link at all. Only a link we do NOT carry
// is worth an account and a page fetch.
//
// Pure but for the three injected dependencies, and free of Svelte and the DOM, so the
// order above is covered by tests rather than by clicking through the box.

import type { ResolvedLink } from './types';

/** What the box should do next. One of these ends every intake run.
 *
 *  `open` is deliberately the outcome of BOTH doors: a posting found in the catalog and
 *  a posting the intake just imported are the same thing to the visitor — a page to go
 *  to — and telling them apart in the UI would be reporting our own plumbing. */
export type LinkIntakeStep =
  | { kind: 'open'; slug: string }
  | { kind: 'signin' }
  | { kind: 'outcome'; resolved: ResolvedLink }
  | { kind: 'error' };

export interface LinkIntakeDeps {
  /** The public lookup. Resolves to null when the catalog holds no such posting. */
  find: (url: string) => Promise<{ public_slug: string } | null>;
  /** The intake. Only ever called for a signed-in visitor. */
  submit: (url: string) => Promise<ResolvedLink>;
  signedIn: () => boolean;
}

/** Run the intake for one pasted link and report what the box should do.
 *
 *  A failure of the public lookup is NOT the end of the run: it is one endpoint being
 *  unavailable, and the intake behind it answers the same question in a slower way. So a
 *  signed-in visitor still gets their link submitted, and only a signed-out one is left
 *  with nothing to show. The alternative — surfacing the lookup's failure — would refuse
 *  a link we could still have resolved. */
export async function runLinkIntake(url: string, deps: LinkIntakeDeps): Promise<LinkIntakeStep> {
  try {
    const found = await deps.find(url);
    if (found) return { kind: 'open', slug: found.public_slug };
  } catch {
    if (!deps.signedIn()) return { kind: 'error' };
  }

  // Not one we carry. Handing it in writes a contribution against an account, so there
  // has to be one — and asking here, rather than before the lookup, is what keeps the
  // find-and-open half open to everybody.
  if (!deps.signedIn()) return { kind: 'signin' };

  try {
    const resolved = await deps.submit(url);
    // The intake carries its own catalog check (a storefront link fronting a posting we
    // already hold under a crawled source), so it can name a posting the public lookup
    // could not. When it does, the page to go to wins over the story of how we got it.
    if (resolved.public_slug) return { kind: 'open', slug: resolved.public_slug };
    return { kind: 'outcome', resolved };
  } catch {
    return { kind: 'error' };
  }
}
