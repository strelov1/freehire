## MODIFIED Requirements

### Requirement: Account-section shell and navigation are translated

The account-section shell (`my/+layout.svelte`) and its navigation labels
SHALL render in the resolved account-section locale, following the same
rules as any other `/my/**` page.

Every section listed in the account navigation SHALL have a translated label
for English and for Russian. A section added to the navigation without one
SHALL fail the test suite rather than silently rendering its English label
inside an otherwise translated sidebar. The label catalog SHALL NOT carry an
entry for a section the navigation no longer lists.

This completeness guarantee applies to English and Russian only. The four
supported-but-not-yet-translated locales (`es`, `pt`, `de`, `fr`) are exempt,
so that they can be translated incrementally.

#### Scenario: Navigation labels follow the resolved locale

- **WHEN** a signed-in user whose account `language` is `ru` opens any
  `/my/**` page
- **THEN** the section navigation's labels render in Russian

#### Scenario: A navigation section added without a translation fails the build

- **WHEN** a new section is added to the account navigation and no Russian
  label is added for it
- **THEN** the test suite fails, naming the section that is missing a label

#### Scenario: A label left behind by a removed section fails the build

- **WHEN** a section is removed from the account navigation but its label
  remains in the catalog
- **THEN** the test suite fails, naming the stale label

## ADDED Requirements

### Requirement: A message catalog may carry translations for any supported locale

The message-catalog mechanism SHALL accept translations for any locale in the
account's supported set, not a fixed pair. Adding a translation for a locale
that has none SHALL require changing only that catalog and the single list of
locales the account section is written in — never the catalog mechanism, the
locale resolvers, or any page.

A locale with no translations in a given catalog SHALL resolve to the English
source, with no visible difference from a locale that has been explicitly
translated to identical text.

#### Scenario: A catalog translated into a third locale resolves that locale

- **WHEN** the account section is declared as written in Spanish, a catalog
  carries English, Russian and Spanish text, and a user whose account
  `language` is `es` opens the page it belongs to
- **THEN** the page renders in Spanish

#### Scenario: A locale absent from a catalog resolves to English

- **WHEN** a catalog carries English and Russian text only, and a user whose
  account `language` is `de` opens the page it belongs to
- **THEN** the page renders in English, with no blank or missing text

#### Scenario: A key omitted from a locale's translation falls back per key

- **WHEN** a catalog's Spanish translation omits one key that its English
  source defines
- **THEN** that one key renders in English and every other key on the page
  renders in Spanish

### Requirement: The rendered language attribute names a language the page is written in

A `/my/**` page SHALL resolve to a non-English locale only when the account
section is actually written in that locale. A locale that `users.language`
accepts but that nothing has been translated into SHALL resolve to English, so
that `<html lang>` never names a language the visible text is not in.

The set of locales the account section is written in SHALL be declared in one
place, so that translating a new locale is that declaration plus the catalogs,
and nothing else.

#### Scenario: A supported but untranslated preference renders and reports English

- **WHEN** a signed-in user whose account `language` is `es` (or `pt`/`de`/`fr`,
  none of which the account section is written in yet) opens a `/my/**` page
- **THEN** the page renders in English **and** its `<html>` element carries
  `lang="en"`

#### Scenario: A corrupt preference renders and reports English

- **WHEN** a signed-in user's stored `language` is a value the application does
  not recognise
- **THEN** the page renders in English and reports `lang="en"` rather than
  failing

### Requirement: Account-section page titles are translated

Every `/my/**` page's browser title SHALL render in the resolved
account-section locale, in the same server-rendered response that carries the
translated page body.

#### Scenario: The browser title follows the resolved locale

- **WHEN** a signed-in user whose account `language` is `ru` opens a
  translated `/my/**` page
- **THEN** the document title renders in Russian

### Requirement: The first batch of account pages is translated

The following account sections SHALL render every user-visible string —
headings, descriptions, form labels, button text, empty states, loading
states and error messages — in the resolved account-section locale:
`/my/submissions`, `/my/api-keys`, `/my/contributions`, `/my/plan`,
`/my/referrals`, and `/my/activity`.

#### Scenario: A translated page renders no English text under a Russian preference

- **WHEN** a signed-in user whose account `language` is `ru` opens one of
  these sections
- **THEN** every heading, label, button and message on the page renders in
  Russian

#### Scenario: A translated page's empty and error states are also translated

- **WHEN** one of these sections has no records to show, or its data request
  fails, for a user whose account `language` is `ru`
- **THEN** the empty-state or error message renders in Russian
