## ADDED Requirements

### Requirement: Support, scheduling and legal-practice tooling is tagged

The skill dictionary SHALL carry canonicals for the tooling named by
administrative, customer-support and immigration-practice postings: `freshdesk`,
`calendly`, `clio`, `uscis`, and the immigration form numbers `i-129` and `i-130`.
`zendesk`, `intercom`, `salesforce`, `hubspot`, `quickbooks` and `notion` already
exist and are unchanged.

#### Scenario: Support desk tooling resolves

- **WHEN** a description names Freshdesk
- **THEN** `jobs.skills` carries the `freshdesk` canonical

#### Scenario: Scheduling tooling resolves

- **WHEN** a description names Calendly
- **THEN** `jobs.skills` carries the `calendly` canonical

#### Scenario: Legal practice tooling resolves when corroborated

- **WHEN** a description names Clio alongside another named tool (e.g. QuickBooks)
- **THEN** `jobs.skills` carries the `clio` canonical

#### Scenario: A bare mention of Clio does not resolve

- **WHEN** a description names only "Clio" with no other named tool present —
  the word is at least as common a first name as other gated canonicals in
  this dictionary (`maya`, `lottie`), and also names the Clio Awards and the
  Renault Clio
- **THEN** `jobs.skills` does not carry the `clio` canonical, the same
  ambiguous-word corroboration rule `houdini` and `maya` already carry

#### Scenario: Immigration forms resolve

- **WHEN** a description names USCIS, Form I-129 or Form I-130
- **THEN** `jobs.skills` carries `uscis`, `i-129` or `i-130` respectively

### Requirement: Every new canonical ships its label and description together

Each canonical added by this change SHALL be committed with its slug in
`dictionaries.go`, its reader-facing label in `labels.go`, and its one-sentence
definition in `descriptions.tsv`, in the same commit. A canonical with no
description fails the build; this requirement records that the rule is met
deliberately rather than discovered.

#### Scenario: A canonical without a description does not build

- **WHEN** a canonical is added to `dictionaries.go` with no `descriptions.tsv` row
- **THEN** the package test fails

#### Scenario: Each new canonical renders a label

- **WHEN** any of `freshdesk`, `calendly`, `clio`, `uscis`, `i-129`, `i-130` is
  rendered to a reader
- **THEN** a curated label is returned rather than the raw slug
