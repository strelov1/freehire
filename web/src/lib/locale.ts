// The account's interface-locale set, mirrored from the backend's CHECK constraint
// on users.language (migrations/0102_user_language.sql / internal/accounts.go's
// supportedLanguages) — keep the two in sync if either changes.
export const SUPPORTED_LOCALES = ['en', 'ru', 'es', 'pt', 'de', 'fr'] as const;

export type Locale = (typeof SUPPORTED_LOCALES)[number];

export function isLocale(value: string | null | undefined): value is Locale {
  return !!value && (SUPPORTED_LOCALES as readonly string[]).includes(value);
}

/** The locales the account section is actually WRITTEN in — a subset of
 *  `SUPPORTED_LOCALES`, which is what `users.language` accepts.
 *
 *  The two differ on purpose. A locale may be offered in the language picker
 *  long before anyone has translated a page into it, and the catalogs
 *  (`$lib/i18n/t`) fall back to English per key, so an untranslated locale
 *  renders English. Resolving it anyway would put a `<html lang>` on that page
 *  naming a language the text is not in — English words announced with Spanish
 *  phonemes by a screen reader — for as long as the copy is missing.
 *
 *  So the resolvers admit only what is listed here, and everything else
 *  collapses to `en`. Translating a new locale is: write its catalogs, then add
 *  it to this array. One place, and the mechanism needs no other change. */
export const TRANSLATED_LOCALES: readonly Locale[] = ['en', 'ru'];

export function isTranslatedLocale(value: string | null | undefined): value is Locale {
  return !!value && (TRANSLATED_LOCALES as readonly string[]).includes(value);
}

/** Non-httpOnly: a rendering preference, not a secret. Synced by the root
 *  layout load from `users.language`; read by hooks.server.ts on every
 *  subsequent request without a DB round trip. */
export const LOCALE_COOKIE = 'hire_lang';
