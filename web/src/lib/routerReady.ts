import { afterNavigate } from '$app/navigation';

/** Run `fn` once, as soon as SvelteKit's router has started — which is what makes
 *  `replaceState`/`pushState` safe to call.
 *
 *  Call it during component initialisation, where `afterNavigate` is allowed. It
 *  replaces `onMount` for mount-time work that reaches for shallow routing, and only
 *  for that: everything else an `onMount` does is already safe there.
 *
 *  Why `onMount` is not safe. SvelteKit hydrates with `root = new app.root({ sync:
 *  false })` — it asks Svelte NOT to flush effects inside the call, precisely so that
 *  nothing runs before it holds the reference. Svelte's legacy client wrapper reads
 *  that flag as `(!options?.props?.$$host || options.sync === false)`, and the left
 *  side is already true for a root with no custom-element host, so it calls
 *  `flushSync()` anyway. Mount-time effects therefore run INSIDE the constructor, one
 *  assignment too early: `replaceState` ends in `root.$set(...)` with `root` still
 *  undefined. In dev a guard catches this first and says "before router is
 *  initialized"; in production that guard is compiled out, so it surfaced instead as
 *  `undefined is not an object (evaluating '$set')` from inside SvelteKit, which named
 *  nothing about the call that caused it.
 *
 *  Why `afterNavigate` alone is not enough either: on the hydrating navigation it
 *  fires a few statements BEFORE SvelteKit sets the flag that dev's guard reads, in
 *  the same synchronous block. The microtask hop is what clears that block — so
 *  `afterNavigate` is here to pin the work to a navigation, and the `await` is here to
 *  make it safe. */
export function onRouterReady(fn: () => void): void {
  let ran = false;
  afterNavigate(async () => {
    if (ran) return;
    ran = true;
    await Promise.resolve();
    fn();
  });
}
