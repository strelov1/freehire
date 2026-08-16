// See https://svelte.dev/docs/kit/types#app.d.ts
declare global {
  namespace App {
    // interface Error {}
    interface Locals {
      // The resolved account-section locale for this request (see
      // hooks.server.ts) — 'en' outside /my/** regardless of the user's
      // preference.
      locale: import('$lib/locale').Locale;
    }
    interface PageData {
      // The current signed-in user, resolved in the root layout's server load
      // and exposed via page data (SSR-safe, per-request). null when signed out.
      user: import('$lib/types').User | null;
      // The resolved account-section locale, synced from `user.language` and
      // path-gated to /my/** in hooks.server.ts — 'en' everywhere else.
      locale: import('$lib/locale').Locale;
    }
    // interface PageState {}
    // interface Platform {}
  }
}

export {};
