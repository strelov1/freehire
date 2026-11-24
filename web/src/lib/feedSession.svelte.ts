import { goto } from '$app/navigation';
import { page } from '$app/state';
import { isUnauthorized } from '#lib/api-core.js';
import { signinUrl } from '#lib/signin.js';

/** Whether the visitor's session ended while Matches was open. Every Matches call that can
 *  hear a 401 reports it here, and the feed shows FeedSessionEnded instead of cards or a state
 *  of its own. Client-only: the cold SSR load of a signed-out visitor is the explainer. */
class FeedSession {
  ended = $state(false);

  /** The session is gone. Ignored once the visitor has left /matches: a late reply must not
   *  mark the next visit, nor send someone who moved on back to sign-in. */
  end = () => {
    if (page.url.pathname === '/matches') this.ended = true;
  };

  /** Whether `e` is the server saying the session is gone (and then Matches takes over the view). */
  lost(e: unknown): boolean {
    if (!isUnauthorized(e)) return false;
    this.end();
    return true;
  }

  /** A fresh visit to the page, or leaving it. */
  reset() {
    this.ended = false;
  }
}

export const feedSession = new FeedSession();

let signingIn: Promise<void> | undefined;

/** Sign in again and come back to /matches, as the rest of the site does for a lost session.
 *  One navigation at a time, however many surfaces ask. Rejects when the navigation is refused. */
export function signInAgainForFeed(): Promise<void> {
  // eslint-disable-next-line svelte/no-navigation-without-resolve -- signinUrl() wraps resolve('/signin'); the rule can't see through the appended query
  signingIn ??= goto(signinUrl({ returnTo: '/matches', mode: 'login' }), { replaceState: true }).finally(() => {
    signingIn = undefined;
  });
  return signingIn;
}
