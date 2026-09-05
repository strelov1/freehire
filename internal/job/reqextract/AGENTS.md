# Requirements extraction conventions

## Scope

Reading a posting's own stated requirements out of its description markup, with no
model call. The second producer of the enrichment contract's `requirements` field; the
first is the LLM in [internal/ai/enrich](../../ai/enrich/AGENTS.md).

## Always true

- **Three vocabularies, and all three are closed.** `requiredHeadings` and
  `preferredHeadings` OPEN a section; `closingHeadings` ENDS one. A heading in none of
  them opens nothing. There is deliberately **no fallback** that infers which list in a
  posting is the requirements list: the most common list in a job posting after the
  requirements is the benefits list, and reading perks as requirements is worse than
  reading nothing.
- **A vocabulary phrase alone is not a heading.** It opens a section only when the rest
  of the line is also vocabulary (`headingTail`). Both of these are real prod headings:
  `Required Qualifications & Skills` heads a requirements list, and
  `MUST HAVE MORNING/DAYTIME AVAILABILITY` heads a list of employee **benefits**. Only
  the tail tells them apart. Prefix matching alone shipped the second one.
- **`closingHeadings` exists because structure cannot decide.** An `h1`–`h6` outside
  every vocabulary closes a section on its tag alone. An inline element cannot:
  `<p>Benefits:</p>` and `<p>You will need:</p>` are the same shape, and a rich-text
  editor writes both. Closing on every unrecognized inline line cost real postings
  their lists; closing on none of them swept the benefits list in. The vocabulary is
  the only thing that separates the two.
- **A wrapper is not a heading.** A `<p>`/`<div>` containing a list is a container,
  however short its text reads. Classifying it as a heading skipped its whole subtree,
  which made whether a posting yielded anything depend on how long its bullets were.
- **One length threshold, in `isHeadingCandidate`.** Short enough to be a title, or
  long enough to be prose, with nothing in between. Prose closes an open section; the
  walk reaches that case only after the heading case has failed, so there is no second
  test to keep in step.
- **Only the FIRST list after a heading belongs to it.** Taking a list closes its
  section.
- **The bound is `enrich.BoundRequirements`, never a local copy.** Both producers of
  the field obey one ceiling, and that function is exported for exactly this. Do not
  restate `maxRequirements` / `maxRequirementTextRunes` here.
- **The output is display material only.** 86% of stored requirement texts are
  distinct (measured on prod 2026-09-04), so this can never be a facet, a filter, or a
  search field. Do not add one.

## How it works

`Derive(descriptionHTML)` parses the fragment with `x/net/html` and walks it in
document order, carrying one piece of state: the priority of the section currently
open, or `""` for none. A recognized heading opens a section, a list closes the one it
was found in, and an unrecognized structural heading, a closing heading, prose or a
table close it too. Entry text is the item's plain text — markup stripped, entities
decoded, whitespace collapsed, a space contributed at each block boundary so
`Go<ul><li>generics` does not read `Gogenerics`.

The result is written to `jobs.requirements_derived` by every job write path (through
`internal/job/job`'s `withDerived`) and folded into the served
`enrichment.requirements` by `jobview.FromDomain` when the model stated none. It is
**not** copied into the `enrichment` blob — see the note in `SetJobEnrichment`
(`internal/platform/db/queries/jobs.sql`) for why a materialised copy is unfixable.

## Extending a vocabulary

Add the phrase, add a test, and **run it against real postings before trusting it**.
Every defect this package has had was found that way and by nothing else: a benefits
list under `MUST HAVE…`, a scheduling note under `Preferred Hours:`, and a perks list
under an inline `Benefits:`. A unit test written from imagination reproduces none of
them.

Coverage after the last such run: **14.0%** of 164 live open postings yield a list.
The regex estimate that motivated the work said 23%; the difference is precision the
vocabulary buys, and it is the right trade — a missed section is a blank space, while
a benefits list under a "Requirements" heading is a false claim the reader cannot
detect.

## Limitations

- **English headings only.** The catalogue is majority English; more languages are an
  additive change to the three vocabularies.
- **No clustering.** Near-duplicate phrasings ("excellent written and verbal
  communication skills" vs "strong written and verbal communication skills") are stored
  as stated. Real, but a different problem.
