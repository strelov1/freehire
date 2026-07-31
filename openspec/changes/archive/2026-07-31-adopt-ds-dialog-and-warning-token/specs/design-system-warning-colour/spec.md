## ADDED Requirements

### Requirement: Caution has one colour, and it is a token

The design system SHALL define a `warning` colour family for caution — the meaning carried
today by ghost-job marks, ATS warnings, match shortfalls and unverified states. App code
SHALL express caution through that family and SHALL NOT choose a hue of its own.

The family SHALL carry the same four roles as `brand`: a solid fill, something legible on
that fill, a text-and-border colour for use on the page background, and a soft tint. One
value cannot serve both the fill and the text — the fill the app uses today is not legible as
text on the page background.

#### Scenario: A caution surface needs a fill

- **WHEN** a surface marks something as needing attention with a solid block of colour
- **THEN** it uses the fill role, and any text on that block uses the foreground role

#### Scenario: A caution surface needs text

- **WHEN** a surface states a caution in words on the page background
- **THEN** it uses the text role, which is legible against that background in both themes

#### Scenario: A new caution surface reaches for a hue

- **WHEN** a file under `web/src` introduces a Tailwind palette utility for caution
- **THEN** the token check fails it, because the file's count rose above its baseline

### Requirement: Dark mode is the token's job, not the call site's

A caution colour SHALL be expressed as one utility. A call site SHALL NOT pair a light
utility with a `dark:` variant to express the same role, because the `.dark` selector already
overrides the token.

#### Scenario: A light/dark pair collapses

- **WHEN** a call site reads `text-amber-700 dark:text-amber-400`
- **THEN** it becomes a single token utility, and the dark variant is removed rather than
  translated
