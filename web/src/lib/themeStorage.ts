// The theme's browser-storage IO, split out of theme.svelte.ts so it can be tested.
//
// Self-contained on purpose, for the reason filterStorage.ts records for itself: it
// feature-detects storage rather than importing `browser` from `$app/environment`, so
// the unit test runs in the plain-Node vitest env. The store beside it cannot be
// exercised there at all — its `$state` field needs a Svelte runtime the env does not
// provide (see paginated.svelte.test.ts) — and a read that no test can reach is how the
// unguarded one below shipped.
//
// Every access is wrapped, and `typeof` sits INSIDE the wrapper rather than in front of
// it: `localStorage` is an accessor that throws `SecurityError` when the document is
// denied storage (a sandboxed iframe, a browser set to block site data), and the throw
// comes from reading the property, so a `typeof` guard placed outside the try throws on
// the guard itself.

export const THEME_KEY = 'hire.theme';

export type ThemeMode = 'light' | 'dark';

/** The stored mode, or `light` when none is recorded or storage is unreachable.
 *  Never throws. */
export function readStoredTheme(): ThemeMode {
  try {
    if (typeof localStorage === 'undefined') return 'light';
    return localStorage.getItem(THEME_KEY) === 'dark' ? 'dark' : 'light';
  } catch {
    return 'light';
  }
}

/** Persist the mode. A no-op when storage is unreachable — the choice then lasts for
 *  this page only. Never throws. */
export function writeStoredTheme(mode: ThemeMode): void {
  try {
    if (typeof localStorage === 'undefined') return;
    localStorage.setItem(THEME_KEY, mode);
  } catch {
    // best-effort: private mode / quota / denied storage
  }
}
