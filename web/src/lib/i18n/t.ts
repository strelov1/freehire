import type { Locale } from '$lib/locale';

// A message catalog: string or string-list leaves, nested per section (e.g.
// `password.heading`, `erased` list items).
export type Messages = { [key: string]: string | string[] | Messages };

type DeepPartial<T extends Messages> = {
  [K in keyof T]?: T[K] extends string | string[] ? T[K] : DeepPartial<Extract<T[K], Messages>>;
};

// Only a plain nested catalog recurses — an array is always a leaf, replaced
// whole by a translation rather than merged element-by-element.
function isMessages(value: unknown): value is Messages {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

// Merges `ru` over a full copy of `en`, per key, at any nesting depth — a key
// missing from `ru` (a section not translated yet) keeps its English value
// instead of the whole section falling back.
function deepMerge<T extends Messages>(en: T, ru: DeepPartial<T> | undefined): T {
  if (!ru) return en;
  const merged = { ...en } as T;
  for (const key of Object.keys(ru) as (keyof T)[]) {
    const ruValue = ru[key];
    const enValue = en[key];
    if (isMessages(enValue) && isMessages(ruValue)) {
      merged[key] = deepMerge(enValue, ruValue as DeepPartial<Messages>) as T[keyof T];
    } else if (ruValue !== undefined) {
      merged[key] = ruValue as T[keyof T];
    }
  }
  return merged;
}

/** Defines a page/section's message catalog: `en` is the source of truth, `ru`
 *  may be partial — any key it omits falls back to the English value. */
export function defineMessages<T extends Messages>(
  en: T,
  ru: DeepPartial<T>,
): { en: T; ru: T } {
  return { en, ru: deepMerge(en, ru) };
}

/** Resolves a catalog for the given locale. Any locale other than `ru` (including
 *  the four supported-but-not-yet-translated ones) renders the English source. */
export function t<T extends Messages>(catalog: { en: T; ru: T }, locale: Locale): T {
  return locale === 'ru' ? catalog.ru : catalog.en;
}
