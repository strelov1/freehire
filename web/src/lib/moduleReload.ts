// One automatic reload when a JS module fails to load and SvelteKit is about to show the
// error page. A browser holding stale cached markup or chunks usually recovers on a fresh
// load; a chunk that is really missing would not, so the reload is allowed once per URL per
// window and the error page stays after that instead of a reload loop.
//
// Takes the storage as a parameter so the guard is testable without a browser; the caller
// passes sessionStorage, which is per tab and gone when the tab closes.

const KEY = 'freehire:module-reload';
const WINDOW_MS = 5 * 60_000;

type Store = Pick<Storage, 'getItem' | 'setItem'>;
type LastReload = { href?: unknown; at?: unknown } | null;

/** True (and recorded) when this URL has not been auto-reloaded within the window. */
export function claimModuleReload(store: Store | undefined, href: string, now: number): boolean {
  if (!store) return false;
  try {
    const last = readLast(store);
    if (last?.href === href && typeof last.at === 'number' && now - last.at < WINDOW_MS) return false;
    store.setItem(KEY, JSON.stringify({ href, at: now }));
    return true;
  } catch {
    // Storage blocked or full (Safari private mode): without a guard, never reload.
    return false;
  }
}

function readLast(store: Store): LastReload {
  try {
    return JSON.parse(store.getItem(KEY) ?? 'null') as LastReload;
  } catch {
    return null; // a corrupted record is no record
  }
}
