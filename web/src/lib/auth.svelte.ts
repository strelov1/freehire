// Auth controller. The session lives in an httpOnly cookie the browser manages,
// so there is no token here. The current user is resolved server-side in the
// root layout load and exposed via `page.data.user` (SSR-safe, per-request — a
// module-level `$state` singleton would leak one request's user into another's
// SSR). Reads go through `currentUser()`/`isAuthenticated()`; the mutations call
// the API and then `invalidateAll()` so the layout re-resolves the user and the
// UI updates.

import { invalidateAll } from '$app/navigation';
import { page } from '$app/state';
import { api } from '$lib/api';
import type { User } from '$lib/types';

/** The current signed-in user, or null. Reactive: reads `page.data.user`. */
export function currentUser(): User | null {
  return page.data.user ?? null;
}

/** True once a session is resolved. Reactive (reads `page.data.user`). */
export function isAuthenticated(): boolean {
  return currentUser() !== null;
}

export async function login(email: string, password: string) {
  await api.login(email, password);
  await invalidateAll();
}

export async function register(email: string, password: string) {
  await api.register(email, password);
  await invalidateAll();
}

export async function logout() {
  // Best-effort: still re-resolve (to signed-out) even if the network call fails.
  try {
    await api.logout();
  } catch {
    // ignore
  }
  await invalidateAll();
}
