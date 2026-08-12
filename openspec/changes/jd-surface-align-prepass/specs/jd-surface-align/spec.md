## Purpose

Deterministically rewrite a tailored CV so its skill wording matches the vacancy's
own spellings before any LLM tailor or autopilot turn, using the skilltag alias
dictionary rather than model inference.

## ADDED Requirements

### Requirement: Preferred surfaces are taken from the vacancy text

The system SHALL, given a vacancy's plain description, resolve each recognised
skill to its canonical slug and record the surface form(s) that appeared in that
description, preserving the vacancy's casing. When more than one surface for the
same canonical appears, the system SHALL prefer the longest surface. When only
one surface appears, that surface is the preferred form even when it is the
shorter alias. Surfaces SHALL come only from the curated skilltag alias
dictionary; unknown phrases SHALL not be invented.

#### Scenario: Long form wins over acronym in the JD

- **WHEN** the vacancy text contains both `IaC` and `infrastructure as code`
- **THEN** the preferred surface for `infrastructure-as-code` is the longer
  `infrastructure as code` form with the vacancy's casing

#### Scenario: JD-only acronym is the preferred surface

- **WHEN** the vacancy text contains `IaC` and does not contain
  `infrastructure as code`
- **THEN** the preferred surface is `IaC` with the vacancy's casing

#### Scenario: Unknown jargon is ignored

- **WHEN** the vacancy text names a tool absent from the skilltag dictionary
- **THEN** no preferred surface is recorded for it

### Requirement: Skills chips and stacks accept any alias of the canonical

The system SHALL rewrite each skills-group item and each experience/project stack
entry that `Canonicalize` resolves to a canonical with a preferred vacancy
surface, replacing that item with the preferred surface. After rewrite, the
system SHALL collapse skills-group items that have become identical.

#### Scenario: Skills chip expands to the JD form

- **WHEN** the tailored CV lists `IaC` in a skills group and the vacancy prefers
  `infrastructure as code`
- **THEN** that skills item becomes `infrastructure as code`

#### Scenario: Skills chip shrinks to the JD acronym

- **WHEN** the tailored CV lists `Infrastructure as Code` and the vacancy prefers
  `IaC`
- **THEN** that skills item becomes `IaC`

#### Scenario: Duplicate chips after rewrite are collapsed

- **WHEN** a skills group lists both `IaC` and `Infrastructure as Code` and the
  vacancy prefers `infrastructure as code`
- **THEN** the group contains that preferred surface once

#### Scenario: An ambiguous chip is still rewritten

- **WHEN** a skills item is `Go` and the vacancy prefers `Golang`
- **THEN** that skills item becomes `Golang`

#### Scenario: No new skill is introduced

- **WHEN** the vacancy prefers `kubernetes` but the tailored CV has no alias of
  that canonical in chips or stacks
- **THEN** the document gains no `kubernetes` (or `k8s`) entry from alignment alone

### Requirement: Prose replace is unambiguous aliases only

The system SHALL rewrite whole-token occurrences in the summary and bullets only
when the matched token is an unambiguous alias of a preferred canonical: a
multi-word phrase alias or a strong acronym. It SHALL NOT rewrite tokens listed
as ambiguous English words in the skilltag dictionary, and SHALL NOT rewrite
tokens of one or two letters. Replacement SHALL use word-boundary matching so an
alias never matches inside a larger word.

#### Scenario: Unambiguous acronym in a bullet is replaced

- **WHEN** a bullet contains the whole token `IaC` and the vacancy prefers
  `infrastructure as code`
- **THEN** that token is replaced and surrounding words are unchanged

#### Scenario: Ambiguous English-word alias in a bullet is left alone

- **WHEN** a bullet contains `go` or `react` as a whole token and the vacancy
  prefers `Golang` or `React`
- **THEN** that bullet is not rewritten for that token

#### Scenario: A short token is not rewritten in prose

- **WHEN** a bullet contains a one- or two-letter whole token that aliases a
  preferred canonical
- **THEN** that token is left unchanged

#### Scenario: A substring that is not a whole token is left alone

- **WHEN** prose contains a longer word that merely embeds an alias spelling
- **THEN** that prose is not rewritten for that alias

#### Scenario: Unambiguous phrase in a bullet is replaced

- **WHEN** a bullet contains `infrastructure as code` and the vacancy prefers
  `IaC`
- **THEN** that phrase is replaced with `IaC`

### Requirement: Alignment is deterministic and idempotent

Applying surface alignment twice to the same document and vacancy text SHALL
leave the document unchanged on the second pass and SHALL require no LLM.

#### Scenario: Second pass is a no-op

- **WHEN** alignment has already rewritten a document to the vacancy's preferred
  surfaces
- **THEN** a second alignment produces an identical document
