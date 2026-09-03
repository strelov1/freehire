// Theme controller. Two explicit modes — `light` / `dark` — persisted in
// localStorage under `hire.theme`. Defaults to `light` regardless of the OS
// preference; `dark` only applies once the user explicitly toggles it. The
// root layout calls `initTheme()` on mount; components read `themeStore` and
// call `setMode(...)`. SSR-safe: every browser API is guarded by `browser`, so
// importing this module on the server (via the header menu) never touches
// window/localStorage. A no-FOUC inline script in app.html applies the class
// before paint (see task 4.2).

import { browser } from '$app/environment';

const STORAGE_KEY = 'hire.theme';

export type ThemeMode = 'light' | 'dark';

function readStored(): ThemeMode {
  if (!browser) return 'light';
  const raw = localStorage.getItem(STORAGE_KEY);
  return raw === 'dark' ? 'dark' : 'light';
}

function apply(mode: ThemeMode) {
  if (!browser) return;
  document.documentElement.classList.toggle('dark', mode === 'dark');
}

class ThemeStore {
  mode = $state<ThemeMode>(readStored());

  isDark = $derived(this.mode === 'dark');

  setMode(next: ThemeMode) {
    this.mode = next;
    if (browser) {
      try {
        localStorage.setItem(STORAGE_KEY, next);
      } catch {
        // best-effort: private mode / quota
      }
    }
    apply(next);
  }

  toggle() {
    this.setMode(this.isDark ? 'light' : 'dark');
  }
}

export const themeStore = new ThemeStore();

/** Re-apply the stored theme. Browser-only (called from the layout's onMount) —
 *  the singleton may have been first constructed on the server, where it
 *  defaults to `light` without reading storage. */
export function initTheme() {
  if (!browser) return;
  themeStore.mode = readStored();
  apply(themeStore.mode);
}
