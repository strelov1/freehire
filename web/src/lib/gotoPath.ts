import { goto } from '$app/navigation';
import { match } from '$app/paths';

/** Navigates to a validated same-origin path (see safeRedirect) that need not be a page
 *  of this app. SvelteKit 3's goto() REJECTS a URL no route matches, and a `returnTo`
 *  can name the Go API: signing in for the CLI or an MCP client returns to
 *  `/api/v1/oauth/authorize?…`, which only a full page load reaches. A bare `void
 *  goto(returnTo)` there swallows the rejection and leaves the visitor on the sign-in
 *  page with nothing happening. */
export async function gotoPath(path: string, opts?: Parameters<typeof goto>[1]): Promise<void> {
  if (await match(path)) {
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- a validated same-origin path, just confirmed to be a route of this app
    return goto(path, opts);
  }
  location.assign(path);
}
