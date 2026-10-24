// Theme controller. Two explicit modes — `light` / `dark` — persisted in
// localStorage under `hire.theme`. Defaults to `light` regardless of the OS
// preference; `dark` only applies once the user explicitly toggles it. The
// root layout calls `initTheme()` on mount; components read `themeStore` and
// call `setMode(...)`. SSR-safe: every browser API is guarded, so importing this
// module on the server (via the header menu) never touches window/localStorage.
// A no-FOUC inline script in app.html applies the class before paint (see task 4.2).
//
// The storage IO lives in #lib/themeStorage rather than here, and the split is not
// cosmetic: `themeStore` is constructed at module scope, so anything that throws on
// the way to it takes the whole module down — and the root layout imports this module.
// That is what an unguarded read of `localStorage` did to visitors browsing with site
// data blocked. The runes in this file put it beyond the plain-Node vitest env, so the
// part that can throw now sits in a file that env can reach, with a test for it.

import { browser } from '$app/env';

import { readStoredTheme, writeStoredTheme, type ThemeMode } from '#lib/themeStorage.js';

export type { ThemeMode };

function apply(mode: ThemeMode) {
  if (!browser) return;
  const root = document.documentElement;
  root.classList.toggle('dark', mode === 'dark');
  // Keep the browser chrome on the page's own background, read from the token rather than
  // restated here (check:tokens holds web to that); app.html carries the pre-paint copy.
  const background = getComputedStyle(root).getPropertyValue('--background').trim();
  if (background) document.querySelector('meta[name="theme-color"]')?.setAttribute('content', background);
}

class ThemeStore {
  mode = $state<ThemeMode>(readStoredTheme());

  isDark = $derived(this.mode === 'dark');

  setMode(next: ThemeMode) {
    this.mode = next;
    writeStoredTheme(next);
    apply(next);
  }

  toggle() {
    this.setMode(this.isDark ? 'light' : 'dark');
  }
}

export const themeStore = new ThemeStore();

/** Re-apply the stored theme. Browser-only (called from the layout's onMount) —
 *  the singleton may have been first constructed on the server, where storage is
 *  unreachable and the mode falls back to `light`. */
export function initTheme() {
  if (!browser) return;
  themeStore.mode = readStoredTheme();
  apply(themeStore.mode);
}
