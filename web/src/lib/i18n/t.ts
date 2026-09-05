import type { Locale } from '$lib/locale';

/** The plural forms of one noun, keyed by CLDR category. `other` is required
 *  because it is the only category every language has.
 *
 *  Built by `plurals()` rather than written as a bare object: the marker is what
 *  tells the merge this is a LEAF and not a nested section, which is what lets a
 *  translation carry categories the English source does not have. */
export type PluralForms = {
  readonly [PLURAL_LEAF]: true;
  other: string;
} & Partial<Record<Intl.LDMLPluralRule, string>>;

const PLURAL_LEAF = '__pluralForms' as const;

/** Marks a set of plural forms as one catalog leaf.
 *
 *  English has two categories and Russian four — how many a noun takes is a
 *  property of the LANGUAGE, not of the message. So unlike every other key, a
 *  translation here is not held to the English source's shape: `{ one, other }`
 *  in English may become `{ one, few, many, other }` in Russian. */
export function plurals(
  forms: { other: string } & Partial<Record<Intl.LDMLPluralRule, string>>,
): PluralForms {
  return { [PLURAL_LEAF]: true, ...forms };
}

function isPluralForms(value: unknown): value is PluralForms {
  return typeof value === 'object' && value !== null && PLURAL_LEAF in value;
}

// A message catalog: string, string-list or plural-forms leaves, nested per
// section (e.g. `password.heading`, `erased` list items).
export type Messages = { [key: string]: string | string[] | PluralForms | Messages };

type DeepPartial<T extends Messages> = {
  // PluralForms is tested first and widened deliberately — see `plurals()`.
  [K in keyof T]?: T[K] extends PluralForms
    ? PluralForms
    : T[K] extends string | string[]
      ? T[K]
      : DeepPartial<Extract<T[K], Messages>>;
};

// The locales a catalog may carry a translation for. `en` is excluded because it
// is the source of truth, not a translation of anything — it is always present.
type TranslatedLocale = Exclude<Locale, 'en'>;

/** A resolved catalog: English always, plus one fully merged copy per locale
 *  that was actually translated. A locale nobody has translated is simply
 *  absent — no English copy is built under its key. */
export type Catalog<T extends Messages> = { en: T } & Partial<Record<TranslatedLocale, T>>;

// Only a plain nested catalog recurses. An array and a plural-forms set are both
// leaves, replaced whole by a translation rather than merged member-by-member —
// for plurals that is the point: merging would leave English's `other` sitting
// under a Russian noun's four forms.
function isMessages(value: unknown): value is Messages {
  return (
    typeof value === 'object' && value !== null && !Array.isArray(value) && !isPluralForms(value)
  );
}

// Merges a translation over a full copy of `en`, per key, at any nesting depth —
// a key the translation omits (a section not translated yet) keeps its English
// value instead of the whole section falling back.
function deepMerge<T extends Messages>(en: T, translation: DeepPartial<T>): T {
  const merged = { ...en } as T;
  for (const key of Object.keys(translation) as (keyof T)[]) {
    const translated = translation[key];
    const source = en[key];
    if (isMessages(source) && isMessages(translated)) {
      merged[key] = deepMerge(source, translated as DeepPartial<Messages>) as T[keyof T];
    } else if (translated !== undefined) {
      merged[key] = translated as T[keyof T];
    }
  }
  return merged;
}

/** Defines a page/section's message catalog. `en` is the source of truth and
 *  stays positional — every translation's shape is inferred from it, so a key
 *  that drifts from the English source is a compile error rather than a widened
 *  type. Each translation may be partial: any key it omits falls back to the
 *  English value, per key rather than per section.
 *
 *  Translating a further locale means adding a key here, never touching this
 *  module. */
export function defineMessages<T extends Messages>(
  en: T,
  translations: Partial<Record<TranslatedLocale, DeepPartial<T>>>,
): Catalog<T> {
  const catalog: Catalog<T> = { en };
  for (const locale of Object.keys(translations) as TranslatedLocale[]) {
    const translation = translations[locale];
    if (translation) catalog[locale] = deepMerge(en, translation);
  }
  return catalog;
}

/** Resolves a catalog for the given locale, falling back to the English source
 *  for any locale this catalog carries no translation for. */
export function t<T extends Messages>(catalog: Catalog<T>, locale: Locale): T {
  return catalog[locale] ?? catalog.en;
}

/** A wire token's display label, falling through to the token itself when the
 *  catalog has no words for it.
 *
 *  Several sections map an API enum — a subscription status, a billing interval,
 *  a metered feature — onto words. The fall-through matters: these tokens come
 *  from a server (and in one case from a payment provider) that may add a value
 *  before the catalog knows it, and a raw `past_due` on screen is recoverable in
 *  a way that a blank is not. */
export function tokenLabel<T extends Record<string, string>>(section: T, id: string): string {
  return section[id] ?? id;
}

/** Picks the plural form for `count` in `locale`.
 *
 *  English gets away with `n === 1 ? x : xs`; Russian does not — 1 обращение,
 *  2 обращения, 5 обращений — so the rule has to come from the locale rather
 *  than from the call site. `Intl.PluralRules` is that rule.
 *
 *  A form the catalog omits falls back to `other`, which for a partially
 *  translated noun means an English word inside a Russian sentence rather than
 *  a blank. That is the same per-key fallback `defineMessages` uses, and it is
 *  visible in exactly the way a missing translation should be. */
export function plural(locale: Locale, count: number, forms: PluralForms): string {
  return forms[new Intl.PluralRules(locale).select(count)] ?? forms.other;
}
