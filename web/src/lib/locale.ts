// The account's interface-locale set, mirrored from the backend's CHECK constraint
// on users.language (migrations/0102_user_language.sql / internal/accounts.go's
// supportedLanguages) — keep the two in sync if either changes.
export const SUPPORTED_LOCALES = ['en', 'ru', 'es', 'pt', 'de', 'fr'] as const;

export type Locale = (typeof SUPPORTED_LOCALES)[number];

export function isLocale(value: string | null | undefined): value is Locale {
  return !!value && (SUPPORTED_LOCALES as readonly string[]).includes(value);
}

/** Non-httpOnly: a rendering preference, not a secret. Synced by the root
 *  layout load from `users.language`; read by hooks.server.ts on every
 *  subsequent request without a DB round trip. */
export const LOCALE_COOKIE = 'hire_lang';
