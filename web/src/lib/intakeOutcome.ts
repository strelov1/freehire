// What we tell somebody who handed us a job link, in words.
//
// The intake answers with five outcomes rather than accepted/rejected (see
// internal/api/handler/intake.go), and every surface that accepts a link has to put the
// same five into the same words. There are two such surfaces on the website now — the
// contribute form and the search box — which is exactly one more than it takes for two
// copies to drift, so the words live here and both render them.
//
// The message is the whole answer. A posting to go to is handled by the caller, because
// only two of the five outcomes carry one and a sentence is not a link.

import type { ResolvedLink } from './types';

/** Put one intake outcome into words.
 *
 *  Nothing here promises a reward. Contributing a board earned an AI credit once; the
 *  currency it was paid in no longer exists (a daily allowance is not something a
 *  one-off act tops up), so the board is still recorded and still attributed, and the
 *  submitter is simply not paid for it. Promising a credit that cannot arrive is worse
 *  than promising nothing. */
export function intakeOutcomeMessage(resolved: ResolvedLink): string {
  switch (resolved.status) {
    case 'found':
      return 'We already have this one.';
    case 'tracked':
      return 'Added — and we already track this company, so the rest of its roles will follow on the next crawl.';
    case 'imported':
      return resolved.company_slug
        ? "Added — we already carry this company, and now we'll crawl this board of theirs too."
        : "Added, and this company is new to us — we'll start crawling its board.";
    case 'review':
      return "Added. Its careers site isn't one we can crawl yet, so we'll check by hand whether we can pull the rest of its jobs.";
    default:
      // queued: nothing could read the page. That says nothing about the board behind
      // it, which was still recorded, so this is a "we'll look" rather than a refusal.
      return "We couldn't read that page. We'll check by hand whether we can pull its jobs.";
  }
}
