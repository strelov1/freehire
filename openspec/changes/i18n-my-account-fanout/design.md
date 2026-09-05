## Context

`i18n-my-account` built the account section's i18n layer and proved it on one
page. Its `design.md` settled the load-bearing questions — hand-rolled catalogs
over a library, a `hire_lang` cookie over per-request session re-derivation,
path-gating in the hook rather than component opt-in, per-key rather than
per-locale fallback. None of that is reopened here.

What is reopened is the one decision that was made for two locales and has to
hold for six: the shape of `defineMessages`. Everything else in this change is
mechanical application of the established pattern, plus a test that makes the
pattern's one failure mode loud.

## Goals / Non-Goals

**Goals**

- Make the catalog API able to hold `es`/`pt`/`de`/`fr` without touching
  `t.ts` again, while the four existing catalogs keep behaving identically.
- Make it impossible for a new `/my/**` nav section to ship without a
  translated label, the way `/my/integrations` did.
- Translate six small account pages, including their `<title>`s, into en/ru.

**Non-Goals**

- Writing `es`/`pt`/`de`/`fr` copy. The issue's sequencing is deliberate:
  translators should work from a settled English source, not a moving one.
- A general lint/CI guard against hardcoded strings anywhere under `/my/**`.
  The nav guard is narrow and cheap because `accountNav` is a closed list; a
  general guard is a much larger design question and stays deferred.
- Reshaping `locale()`, the cookie, the hook, or `<html lang>` handling.

## Decisions

### `defineMessages(en, translations)` — a locale map, with `en` still positional

The alternative shapes considered:

1. `defineMessages({ en, ru, es })` — one object, every locale a peer.
2. `defineMessages(en, { ru, es })` — `en` positional, the rest a map.
3. Keep `defineMessages(en, ru)` and add `defineMessagesN` beside it.

Option 2 wins. `en` is not a peer of the others: it is the source of truth, and
its type `T` is what every translation's `DeepPartial<T>` is derived from.
Folding it into the same object makes TypeScript infer `T` from the union of all
locales, which turns a typo in a Russian key from a compile error into a widened
type. Keeping `en` positional preserves the inference that already catches
drift between the English source and a translation.

Option 3 was rejected on principle rather than on cost: two helpers with
overlapping jobs is exactly the "second answer" problem the repository's
conventions warn about elsewhere. Four call sites is a cheap migration.

### A locale with no translations is never materialised

The current `defineMessages` eagerly deep-merges `ru` over `en` at module-eval
time and returns `{ en, ru }`. Extending that naively to six locales would
build four extra full copies of every catalog, all of them byte-identical to
`en`, for locales nobody has translated yet — across ~20 catalogs.

Instead the returned catalog is `{ en: T } & Partial<Record<Locale, T>>`, and
`t()` resolves `catalog[locale] ?? catalog.en`. A locale absent from the map
costs one property lookup and one nullish coalesce. The observable behaviour is
identical to today's — `t(catalog, 'es')` returns the English strings — but by
falling back rather than by having pre-built an English copy under the `es` key.

This does mean `t(catalog, 'es') === t(catalog, 'en')` by identity, where today
they are equal but distinct objects. Nothing depends on that; the existing
`t.test.ts` assertions use `toEqual`, which holds either way.

### The nav guard asserts `en` and `ru` completeness, and nothing more

`shell.ts` keys nav labels by href precisely so a missing entry degrades to the
English literal from `accountNav.ts` instead of rendering blank. That fallback
is right and stays. But it also means the only signal that a section was
forgotten is a human noticing one English word in a Russian sidebar — which,
for `/my/integrations`, took a fortnight and an unrelated audit.

So the test asserts two directions over the closed `accountNav` list:

- every href has an entry in both the `en` and the `ru` navItems map;
- every navItems key is an href `accountNav` still has (catching a stale entry
  left behind when a route is renamed or a section removed).

`es`/`pt`/`de`/`fr` are exempt by construction: they have no navItems map at
all, and asserting their completeness would forbid exactly the incremental
translation this change is making possible.

### `<svelte:head><title>` joins the catalog

`/my/security` already translates its title via a `headTitle` key. Every other
account page hardcodes English in its `+page.svelte` wrapper. Since those
wrappers are thin (most are ~13 lines: a `<title>`, a max-width div, and the
view component), the catalog lives with the **view** component that owns the
page's other strings, and the wrapper imports it for the title alone. One
catalog per page, not two.

### Plural forms are a catalog leaf, not a nested section

`/my/plan` renders "5 model calls" from a count. English gets away with
`n === 1 ? x : xs`; Russian does not — 1 обращение, 2 обращения, 5 обращений,
and 21 takes the singular while 11 does not. A hand-rolled check is wrong in a
way that is invisible to anyone reading the English source.

`Intl.PluralRules` is the rule, so `plural(locale, count, forms)` reads it. What
needed designing was the *catalog* side. Every other key holds a translation to
the English source's shape — that inference is what catches drift. But how many
categories a noun takes is a property of the LANGUAGE, so English's
`{ one, other }` must be allowed to become Russian's `{ one, few, many, other }`.

So a form set is built by `plurals()`, which marks it as a leaf: `DeepPartial`
widens it to `PluralForms` instead of pinning it to the source's shape, and
`deepMerge` replaces it whole rather than merging into it. Merging would be the
subtle failure — English's `other` surviving under a Russian noun means a count
of 2 reads "2 model calls" mid-sentence.

The alternative — declaring every English noun with all four categories — was
rejected: it puts dead keys on every catalog to describe a language that has no
word for them.

### Page catalogs live with the component that renders the strings

Most `/my/**` routes are thin wrappers around a `$lib/components/*View.svelte`.
Following the established convention, a route-owned catalog is `messages.ts`
next to `+page.svelte`; a component-owned one is `Foo.messages.ts` next to
`Foo.svelte` (as `DeleteAccountButton.messages.ts` already does). For these six
pages the strings are all in the view, so the catalog goes beside the view.

`/my/activity` is the exception: it composes four sibling components
(`AnalysesView`, `Hidden`, `JobHistory`, `SavedJobs`) plus its own tab strip.
Each component gets its own colocated catalog; the tab strip's labels go in a
route-level `messages.ts`, mirroring how `/my/profile` already handles its tabs.

## Known gaps this change does not close

Each is a whole surface of its own, and each is visible on a translated page —
recorded here so the next pass starts from a list rather than from a bug report.

- **Dates and relative times follow the BROWSER's locale, not the account's.**
  `timeAgo` and `formatDateOrAgo` (`web/src/lib/utils.ts`) and the page-local
  `toLocaleDateString(undefined, …)` helpers all pass `undefined`, which means
  the browser. So a Russian page can read "3 days ago" on an English-locale
  browser, and — the mirror image — a public English page can read "3 дня
  назад". Threading the resolved locale through would touch every call site
  including the public pages, which is why it is not folded in here.
- **Strings the SERVER composes stay English.** An `ApiError`'s message
  (`ContributeView`'s malformed-link path), the plan history's `label`/`subtitle`,
  and an analysis `verdict` are all sentences the API sends. Translating them
  is a backend change, not a catalog one.
- **`JobRow` is not translated.** The 546-line posting card is shared with the
  public feed and the highest-traffic surface in the app; folding it in would
  double this change and put its risk on `/jobs`. So `/my/activity` renders
  translated chrome around English job cards, which is the honest state until
  that card is taken on its own.
- **`es`/`pt`/`de`/`fr` have no copy.** The mechanism now holds them and
  `TRANSLATED_LOCALES` gates them; what is missing is the words.

## Risks / Trade-offs

- **The four call sites migrate in one commit.** `defineMessages`'s signature
  change is not backward compatible, so `shell.ts`, `security/messages.ts`,
  `profile/messages.ts` and `DeleteAccountButton.messages.ts` all move with it.
  This is the entire reason for doing it now rather than after the fan-out:
  the same commit against ~20 catalogs is a diff nobody reviews properly.
- **The nav guard only covers the nav.** A page whose body strings were never
  translated still passes every check. That is the accepted boundary — a
  general hardcoded-string guard is deferred, and the nav is guarded because it
  is a closed list that every other section depends on, not because it is the
  only place drift can happen.
- **Six pages of Russian copy land without a native review pass.** Same
  trade-off `i18n-my-account` accepted for `/my/security`. The per-key fallback
  means a later copy correction is a one-line edit, not a re-migration.
- **`/my/activity`'s components are shared.** `SavedJobs` and `JobHistory` may
  render outside `/my/**`. Per the scope-boundary rule this is safe:
  `locale()` reads `page.data.locale`, which `hooks.server.ts` forces to `en`
  on every non-`/my/**` path, so those components render English there
  unconditionally. Each such component's placement is verified during its task
  rather than assumed.

## Migration Plan

Phase 1 (the API and the guard) must land before Phase 2, because Phase 2's new
catalogs are written in the new signature. Within Phase 2 the pages are
independent and can be split across PRs if the diff runs large; the suggested
split is the four simplest (`submissions`, `api-keys`, `contributions`, `plan`)
first, then `referrals` and `activity`.

No data migration, no deploy ordering constraint, no feature flag: the change
is entirely within the SvelteKit app's render path, and a user's resolved
locale is unchanged by it.
