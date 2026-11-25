## MODIFIED Requirements

### Requirement: Curated collections are a company-level membership fact

The system SHALL model a curated collection as a company-level fact: each company
MAY belong to zero or more collections, stored as a set of collection slugs on the
company. A collection slug SHALL come from a fixed, code-owned registry. Each
registry entry SHALL carry a `slug`, a human `title`, a `description`, a `kind`,
and a membership source — exactly one of a static hand list of canonical company
slugs or a remote dataset. Adding a collection SHALL be a single registry entry.
Membership SHALL NOT be derivable from a job's text or its ATS source — it is a
fact about the company, populated only from the registry's sources.

The `kind` SHALL distinguish an **editorial** collection (a curated theme, such as
Big Tech or Unicorns), a **credential** (a verifiable fact drawn from an
authoritative public register), and a **backer** (the accelerator or fund that
selected the company). The kind SHALL be part of the registry contract shared with
the frontend, because it determines how a tag is presented; it SHALL NOT be
hand-mirrored in a second place where it could drift from the Go registry.

A dataset SHALL be defined by a parser yielding **records** rather than bare names.
A record SHALL carry the member's name and MAY carry source metadata (for example a
route, a locality, or a registry identifier) for later gating. A dataset SHALL
supply its payload by exactly one of a fixed URL, an embedded blob, a resolver that
determines the URL at fetch time, or a self-fetching source that produces records
directly — the resolver for a source whose published URL changes between snapshots,
the self-fetching source for one no single URL can express, such as a paginated
directory. A self-fetching source SHALL be responsible for reading its source
completely; a partial read SHALL be an error rather than a smaller membership,
because a truncated fetch is indistinguishable from a shrunken source and would
reconcile the tag off every company it failed to read.

A registry entry MAY carry a **gate**: a predicate over the candidate company and
the matched record that SHALL hold before the tag is applied. An entry with no gate
SHALL be matched on name alone, unchanged from prior behaviour.

#### Scenario: A company belongs to multiple collections

- **WHEN** a company qualifies for two collections (e.g. `yc` and `bigtech`)
- **THEN** the company's collection set contains both slugs

#### Scenario: The registry defines each collection's display copy, kind, and source

- **WHEN** the collection registry is read
- **THEN** each entry exposes a slug, title, description, kind, and exactly one
  membership source (a static slug list or a dataset)

#### Scenario: A dataset parser yields records carrying source metadata

- **WHEN** a dataset whose source publishes per-member attributes is parsed
- **THEN** each record exposes the member's name plus that source's metadata,
  available to the entry's gate

#### Scenario: A gateless entry matches on name alone

- **WHEN** a registry entry defines no gate
- **THEN** every name-matched company is tagged, with no additional condition
  applied

#### Scenario: A dataset resolves its URL at fetch time

- **WHEN** a dataset defines a resolver instead of a fixed URL
- **THEN** the URL is determined during the run and the payload is fetched from it

#### Scenario: A self-fetching dataset produces records directly

- **WHEN** a dataset defines a self-fetching source instead of a URL, blob or resolver
- **THEN** it is invoked during the run and returns the membership records itself

#### Scenario: A dataset declaring two payload forms is rejected

- **WHEN** a dataset declares both a self-fetching source and any of a URL, blob or
  resolver
- **THEN** the registry is invalid and the run refuses to proceed
