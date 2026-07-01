// Theme controller. Three modes — explicit `light` / `dark`, or `system` which
// tracks the OS preference. Persisted in localStorage under `hire.theme`.
// The root layout calls `initTheme()` on mount; components read `themeStore` and
// call `setMode(...)`. SSR-safe: every browser API is guarded by `browser`, so
// importing this module on the server (via the header menu) never touches
// window/localStorage. A no-FOUC inline script in app.html applies the class
// before paint (see task 4.2).

import { browser } from '$app/environment';

const STORAGE_KEY = 'hire.theme';

export type ThemeMode = 'light' | 'dark' | 'system';

const mq = browser ? window.matchMedia('(prefers-color-scheme: dark)') : null;

function readStored(): ThemeMode {
  if (!browser) return 'system';
  const raw = localStorage.getItem(STORAGE_KEY);
  if (raw === 'light' || raw === 'dark' || raw === 'system') return raw;
  return 'system';
}

function apply(mode: ThemeMode) {
  if (!browser || !mq) return;
  const dark = mode === 'dark' || (mode === 'system' && mq.matches);
  document.documentElement.classList.toggle('dark', dark);
}

class ThemeStore {
  mode = $state<ThemeMode>(readStored());
  /** Live OS `prefers-color-scheme: dark` state, kept current by `initTheme`. */
  systemDark = $state(mq ? mq.matches : false);

  /** Effective dark state — explicit `dark`, or `system` resolving to the OS. */
  isDark = $derived(this.mode === 'dark' || (this.mode === 'system' && this.systemDark));

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

  /** Binary flip: light <-> dark. `system` collapses to its effective value
   *  first, so the first click always yields a concrete choice. */
  toggle() {
    this.setMode(this.isDark ? 'light' : 'dark');
  }
}

export const themeStore = new ThemeStore();

/** Apply the stored theme and keep `system` mode tracking the OS preference.
 *  Browser-only (called from the layout's onMount). */
export function initTheme() {
  if (!browser || !mq) return;
  // Re-sync from storage in case the singleton was first constructed on the
  // server (mode defaulted to 'system' there).
  themeStore.mode = readStored();
  themeStore.systemDark = mq.matches;
  apply(themeStore.mode);
  const onChange = () => {
    themeStore.systemDark = mq.matches;
    if (themeStore.mode === 'system') apply('system');
  };
  mq.addEventListener('change', onChange);
  if (import.meta.hot) {
    import.meta.hot.dispose(() => mq.removeEventListener('change', onChange));
  }
}
