import { ApiError, api } from '#lib/api.js';
import { forgetRecentAuthExpiry, startProviderTrip, takeProviderLinkTrip } from '#lib/recentAuth.js';

// Connecting a sign-in provider to the signed-in account (auth v2 purpose `link`). It rides
// the re-authentication transport in recentAuth.ts and ends on the same /my/reauth callback,
// but lives here so the pages that only ask for confirmation do not ship it.

/** Leave for `provider` to attach it to the signed-in account. The server spends a recent-auth
 *  proof on the link, so the caller makes sure one is held first (`ConfirmIdentity.prove()`).
 *  The trip ends on /my/reauth, which hands the code to `completeProviderLink`. */
export async function beginProviderLink(provider: string): Promise<void> {
  await startProviderTrip(provider, 'link', '/my/account');
}

/** How a link trip ended: the provider attached, or the server's refusal code
 *  (`identity_in_use`, `provider_already_linked`, `recent_auth_required`, …) for the account
 *  page to word. Never throws: every ending goes back to that page. */
type LinkOutcome = { linked: string } | { error: string };

export async function completeProviderLink(code: string | null): Promise<LinkOutcome> {
  const verifier = takeProviderLinkTrip();
  // No code: the provider refused or the member declined consent.
  if (!code) return { error: 'cancelled' };
  if (!verifier) return { error: 'expired' };
  try {
    const { provider } = await api.exchangeOAuthLink(code, verifier);
    // The link spent the proof server-side; a hint that it is still held would let the next
    // gated action skip the question and meet a 428.
    forgetRecentAuthExpiry();
    return { linked: provider };
  } catch (e) {
    if (!(e instanceof ApiError)) return { error: 'failed' };
    if (e.status === 428) forgetRecentAuthExpiry();
    return { error: typeof e.body?.code === 'string' ? e.body.code : 'failed' };
  }
}
