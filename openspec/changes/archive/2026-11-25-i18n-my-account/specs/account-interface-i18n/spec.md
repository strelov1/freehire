## Purpose

Renders the signed-in account section (`/my/**`) in the user's preferred
interface language, resolved from their existing `users.language` account
setting, while every other route in the app stays English regardless of that
setting.

## ADDED Requirements

### Requirement: Account-section locale follows the user's language preference

Every page under `/my/**` SHALL render its interface text (headings, labels,
button text, and error messages within scope of this change) in the
account's `language` preference when that preference is `en` or `ru`. For
any other supported value of `language` (`es`, `pt`, `de`, `fr`), the account
section SHALL render in English rather than showing untranslated or missing
text.

#### Scenario: Signed-in account section respects a Russian preference

- **WHEN** a signed-in user whose account `language` is `ru` opens
  `/my/security`
- **THEN** the page's headings, labels, and messages render in Russian

#### Scenario: An unsupported-for-translation preference falls back to English

- **WHEN** a signed-in user whose account `language` is `es` (or `pt`/`de`/`fr`)
  opens `/my/security`
- **THEN** the page renders in English, with no blank or missing text

#### Scenario: Default English preference renders English

- **WHEN** a signed-in user whose account `language` is `en` (the default)
  opens `/my/security`
- **THEN** the page renders in English

### Requirement: Public routes are never translated

Routes outside `/my/**` SHALL always render in English, regardless of the
signed-in user's `language` preference, and SHALL always report
`<html lang="en">`.

#### Scenario: A public page ignores a non-English preference

- **WHEN** a signed-in user whose account `language` is `ru` opens a public
  page such as `/jobs`
- **THEN** the page renders in English and the response's `<html>` element
  carries `lang="en"`

### Requirement: The rendered `<html lang>` attribute matches the resolved locale

Any `/my/**` page's initial server-rendered HTML SHALL carry an `<html lang>`
attribute matching the resolved account-section locale (`en` or `ru`), correct
on first byte without waiting for client-side hydration.

#### Scenario: First response already carries the correct language attribute

- **WHEN** a signed-in user whose account `language` is `ru` requests
  `/my/security` directly (full page load, not a client navigation)
- **THEN** the initial HTML response's `<html>` element carries `lang="ru"`

### Requirement: Changing the language preference updates the account section without a reload

When a signed-in user changes their account `language` preference (via the
existing language picker), every currently-open `/my/**` page's translated
text SHALL update to the new language without a full page reload.

#### Scenario: Switching language updates an open account page live

- **WHEN** a signed-in user on `/my/security` changes their language
  preference from English to Russian using the account language picker
- **THEN** the security page's visible text switches to Russian without the
  browser performing a full page reload

### Requirement: Account-section shell and navigation are translated

The account-section shell (`my/+layout.svelte`) and its navigation labels
SHALL render in the resolved account-section locale, following the same
rules as any other `/my/**` page.

#### Scenario: Navigation labels follow the resolved locale

- **WHEN** a signed-in user whose account `language` is `ru` opens any
  `/my/**` page
- **THEN** the section navigation's labels render in Russian
