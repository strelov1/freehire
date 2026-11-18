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
export function signinUrl(
  opts: {
    returnTo?: string;
    cancelTo?: string;
    error?: string;
    mode?: 'login';
  } = {},
): string {
  const params = new URLSearchParams();
  if (opts.returnTo) params.set('returnTo', opts.returnTo);
  if (opts.cancelTo) params.set('cancelTo', opts.cancelTo);
  if (opts.error) params.set('error', opts.error);
  if (opts.mode) params.set('mode', opts.mode);
  const query = params.toString();
  return `${resolve('/signin')}${query ? `?${query}` : ''}`;
}

/** The URL for /onboarding carrying `returnTo` — where the flow sends a signed-in
 *  visitor who skips it or finishes it. Every entry point (the layout's once-per-account
 *  redirect, the feed's onboarding nudge) builds this the same way, so they share one
 *  place to change the query param name or add validation. */
export function onboardingUrl(returnTo: string): string {
  return `${resolve('/onboarding')}?returnTo=${encodeURIComponent(returnTo)}`;
}

/** The gate every in-place "sign in to do X" action uses (Save, Tailor, Follow, Vote, Reply,
 *  and the rest). A guest is sent to /onboarding FIRST, carrying the page they were on (or an
 *  explicit `returnTo`): the flow's own steps are the account path (drop a CV and sign up from
 *  the results, skip it, or follow "Sign in" if they already have an account), and every exit
 *  of it comes back to `returnTo`. There is no resume — the action itself is not retried (and
 *  nothing paid or model-backed ever fires on its own), so the visitor clicks again once
 *  they're back.
 *
 *  Already on /onboarding or /signin, it goes straight to the sign-in form: sending a visitor
 *  to the page they are on, or from the credential page to the flow that sent them there,
 *  would be a loop. An explicit "Sign in" control (the header, a "Sign in" link) is not an
 *  action gate and uses {@link promptLogin}. */
export function promptSignIn(): void {
  promptSignInFor(page.url.pathname + page.url.search);
}

/** {@link promptSignIn} for a page whose address `page.url` does not yet show (a shallow-routed
 *  tab or slot choice): the caller names where the visitor should come back to. */
export function promptSignInFor(returnTo: string): void {
  if (page.url.pathname === '/onboarding' || page.url.pathname === '/signin') return promptLogin();
  // eslint-disable-next-line svelte/no-navigation-without-resolve -- onboardingUrl() wraps resolve('/onboarding'); the rule can't see through the appended ?returnTo= query
  void goto(onboardingUrl(returnTo));
}

/** The explicit sign-in: straight to the sign-in form, back to the page the visitor was on. */
export function promptLogin(): void {
  // eslint-disable-next-line svelte/no-navigation-without-resolve -- signinUrl() wraps resolve('/signin'); the rule can't see through the appended query
  void goto(signinUrl({ returnTo: page.url.pathname + page.url.search, mode: 'login' }));
}
