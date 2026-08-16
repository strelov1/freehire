import { page } from '$app/state';

// Reads the resolved account-section locale reactively from `page.data` (set by
// the root layout load, path-gated to /my/** in hooks.server.ts) — the same
// per-request-safe pattern `currentUser()` uses in auth.svelte.ts, not a
// module-level store.
export function locale() {
  return page.data.locale;
}
