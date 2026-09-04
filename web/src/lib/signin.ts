import { goto } from '$app/navigation';
import { page } from '$app/state';
import { resolve } from '$app/paths';

/** The URL for /signin carrying whichever of these a caller has: `returnTo` (where a
 *  successful sign-in/register sends the visitor — defaults to /signin's own '/'
 *  fallback when omitted), `cancelTo` (where the close button goes, if different from
 *  `returnTo` — see /signin's own doc comment on why those are separate), `error`
 *  ('oauth' for a failed OAuth callback — shown as an inline message), and `mode`
 *  ('login' to open the sign-in form instead of the register one). Every caller that
 *  sends a visitor to /signin builds the URL here, so the query param names can't
 *  drift between them.
 *
 *  `mode` is a QUERY param, not the URL hash /signin itself uses to name the screen
 *  once landed there (see that page's own doc comment) — a fragment never reaches
 *  the server, so a caller-requested mode has to arrive as a query param or the
 *  server-rendered HTML would show the wrong screen until client JS corrects it
 *  after hydration. /signin promotes it into the hash on arrival (its own
 *  `syncHash` canonicalization), so the address bar still ends up on #login. */
export function signinUrl(opts: {
  returnTo?: string;
  cancelTo?: string;
  error?: string;
  mode?: 'login';
} = {}): string {
  const params = new URLSearchParams();
  if (opts.returnTo) params.set('returnTo', opts.returnTo);
  if (opts.cancelTo) params.set('cancelTo', opts.cancelTo);
  if (opts.error) params.set('error', opts.error);
  if (opts.mode) params.set('mode', opts.mode);
  const query = params.toString();
  return `${resolve('/signin')}${query ? `?${query}` : ''}`;
}

/** The gate every in-place "sign in to do X" action uses (Save, Follow, Vote, Reply,
 *  and the rest): send the visitor to /signin's sign-in form, returning to the page
 *  they were on. There is no resume — the action itself is not retried, so the
 *  visitor clicks again once they're back (matches the behavior when this was an
 *  in-place dialog: signing in never auto-continued the click that opened it). */
export function promptSignIn(): void {
  // eslint-disable-next-line svelte/no-navigation-without-resolve -- signinUrl() wraps resolve('/signin'); the rule can't see through the appended query
  void goto(signinUrl({ returnTo: page.url.pathname + page.url.search, mode: 'login' }));
}
