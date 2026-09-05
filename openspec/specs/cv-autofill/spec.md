# cv-autofill Specification

## Purpose
TBD - created by archiving change cv-autofill-onboarding. Update Purpose after archive.
## Requirements
### Requirement: Résumé extraction returns a structured profile from dictionaries

The system SHALL derive a résumé's skills, seniority, and the categories it spans from the uploaded résumé using only the existing deterministic dictionaries, and MUST NOT guess — a grade it cannot resolve is omitted and the category set is empty when nothing resolves.

Skills come from the skilltag dictionary over the whole résumé (skills appear anywhere). Seniority and categories come from the classify dictionary over the résumé's *headline* — the leading title and summary words, with contact/metadata tokens (email, phone, profile URL, bare numbers, punctuation) dropped and whitespace collapsed so a PDF that extracts one token per line still reaches the title, while the career-history section below cannot reach in and over-promote the grade. Categories are every category the headline mentions (a person can be several — backend and ML), distinct and in precedence order. The extraction uses no LLM, so it stays instant and does not depend on the LLM configuration. `skills` and `categories` are always arrays (empty when nothing resolves); `seniority` is omitted when unresolved. The résumé is stored and embedded as before, and the endpoint remains authenticated.

#### Scenario: A résumé yields the fields the dictionaries resolve

- **WHEN** a signed-in user submits a résumé that names a recognizable role and known skills
- **THEN** the response includes the canonical skills, the resolved seniority, and every category the headline resolves

#### Scenario: A résumé spanning several functions returns every category

- **WHEN** the résumé's headline names more than one specialization (e.g. backend and data engineering)
- **THEN** the response's categories include all of them, in precedence order

#### Scenario: Unresolved fields are omitted or empty, never guessed

- **WHEN** the dictionaries cannot resolve a field from the résumé (e.g. no recognizable seniority)
- **THEN** seniority is omitted and categories is empty, while the fields that did resolve are returned

#### Scenario: Extraction does not require the LLM

- **WHEN** the LLM integration is not configured
- **THEN** résumé extraction still returns skills and any dictionary-resolved seniority/categories

#### Scenario: Existing skills callers are unaffected

- **WHEN** a client that reads only the skills field submits a résumé
- **THEN** the skills are returned exactly as before, regardless of the added fields

### Requirement: The onboarding wizard can pre-fill from a résumé

The system SHALL offer a résumé-upload path in the onboarding wizard that pre-fills the wizard's focus (categories) and seniority — both multi-select — and stack (skills) from the extraction, and SHALL stay on the current step so the user reviews the pre-filled pills rather than being advanced past them. Work mode and region are left to the user, since a résumé does not state them.

Because résumé extraction is authenticated, an unauthenticated visitor is prompted to sign in before uploading. A failed or empty extraction never blocks the wizard: the visitor continues with manual entry and a short note reports what was (or was not) filled. Only the fields the extraction resolved are pre-filled, merged into any manual selection without dropping it; the rest stay empty for the user to pick.

#### Scenario: Upload pre-fills in place for review

- **WHEN** a signed-in visitor uploads a résumé in the wizard
- **THEN** the wizard pre-fills the resolved focus (possibly several), seniority, and stack, and stays on the current step so the visitor can review and correct them

#### Scenario: Anonymous visitor is asked to sign in first

- **WHEN** an unauthenticated visitor chooses the résumé-upload path
- **THEN** they are prompted to sign in before the résumé is uploaded

#### Scenario: A failed extraction falls back to manual

- **WHEN** the résumé cannot be read or resolves nothing
- **THEN** a note or error is shown and the visitor can continue configuring the feed manually

### Requirement: The profile form pre-fills its specializations from a résumé

The system SHALL, on the profile page, pre-fill the profile's specializations from the résumé's resolved categories — reusing the same extraction that already fills the profile's skills, respecting the specialization cap — so a single upload populates the whole profile.

#### Scenario: Résumé upload fills specializations and skills

- **WHEN** a user uploads a résumé on the profile page
- **THEN** the résumé's resolved categories are merged into the profile's specializations (up to the cap) and its skills into the profile's skills

### Requirement: The wizard pre-fills years of experience and profile links from the résumé

The system SHALL pre-fill the wizard's years-of-experience step and its profile-links step
from the uploaded résumé's extraction, which already yields a total-years figure and a list
of links, so the candidate confirms a fact already on their CV instead of retyping it.

A link is offered against a named field by recognising its destination — a LinkedIn member
URL fills the LinkedIn field, a GitHub URL fills the GitHub field — rather than by asking the
candidate to sort the extracted list themselves. Links the extraction found but could not
recognise MUST be preserved, not dropped.

A pre-filled value is a suggestion, not an answer: it MUST remain editable and clearable, and
it MUST be recorded as the candidate's own only once they pass through the step that offers
it. An extraction that yields no total-years figure and no links MUST leave both steps empty
rather than blocking or skipping them.

#### Scenario: Total years is offered for confirmation

- **WHEN** a user uploads a résumé from which a total-years figure is extracted
- **THEN** the years-of-experience step opens with that figure pre-filled and editable

#### Scenario: Extracted links land in their named fields

- **WHEN** an extraction yields a LinkedIn member URL and a GitHub URL
- **THEN** the LinkedIn field is pre-filled with the former and the GitHub field with the latter

#### Scenario: Unrecognised links survive

- **WHEN** an extraction yields a personal site URL alongside a LinkedIn URL
- **THEN** the LinkedIn field is pre-filled and the personal site URL is still retained

#### Scenario: An extraction yielding neither leaves the steps empty

- **WHEN** an extraction yields no total-years figure and no links
- **THEN** both steps are presented empty and remain answerable by hand

#### Scenario: A correction replaces the suggestion

- **WHEN** a user edits a pre-filled years figure before leaving the step
- **THEN** their edited value is stored, and a later résumé re-upload does not overwrite it
