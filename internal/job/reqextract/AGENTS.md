# Requirements extraction conventions

## Scope

Reading a posting's own stated requirements out of its description markup, with no
model call. The second producer of the enrichment contract's `requirements` field; the
first is the LLM in [internal/ai/enrich](../../ai/enrich/AGENTS.md).

The heading vocabulary has a second consumer: `MaskPreferred` lends it to
`internal/job/jobfacts`, so the deterministic fact matchers know where a nice-to-have
section ends instead of each guessing.

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
- **`MaskPreferred` blanks, it never deletes.** Its callers read punctuation as
  structure — `jobfacts.EnglishLevel` binds a level word to an English keyword only
  when no `.` or newline separates them — so a pass that cut a span out and rejoined
  what surrounded it would introduce a sentence boundary the posting never wrote. That
  is not hypothetical: the shape this replaced dropped the level from `English, B2
  required`, one of the commonest phrasings there is. Replacing letters and digits with
  spaces (`BlankWords`) leaves every boundary where the posting put it, and a
  description with no preferred section comes back **byte-identical**, re-render
  skipped.
- **`MaskPreferred` closes a section only on a heading; `Derive` also closes on prose.**
  The difference is deliberate and is the whole reason they are two walks over one
  vocabulary. `Derive` wants the LIST under a heading, so prose between the two means
  the list is no longer that heading's. A preferred SECTION is preferred throughout,
  paragraphs included — which is the shape the defect was reported against. The cost is
  that a preferred section a posting never closes masks the rest of the description;
  that understates requirements rather than overstating them, which is the direction a
  reader can detect.
- **`headingDecision` reports whether it RECOGNIZED the heading, and `MaskPreferred`
  needs the answer.** An unrecognized short line inside a preferred section is that
  section's CONTENT. Treating it as a title (which `Derive` may, since a title is never
  an item either way) leaves its words unmasked — that bug shipped in the first draft
  and is what `PhD.` in a `<p>` slipped through.
- **A preferred heading's own words are blanked with its section.** Leaving `Nice to
  have` legible puts the marker phrase back into the text for `jobfacts`' second,
  clause-level pass to find, which then blanks the sentence around it — requirements and
  all. This cost a required `CISSP` in test before it was caught.
- **The bound is `enrich.BoundRequirements`, never a local copy.** Both producers of
  the field obey one ceiling, and that function is exported for exactly this. Do not
  restate `maxRequirements` / `maxRequirementTextRunes` here.
- **86% of stored requirement texts are distinct** (measured on prod 2026-09-04), so
  this can never be a facet, a filter, or a search field. Do not add one.
- **The job page does NOT render this list, and adding a section that does is a
  mistake already made once.** The extractor fires exactly when the description
  carries a `Requirements` heading with a list under it — which is exactly when the
  page already SHOWS that list, in the description, a few centimetres higher. A
  section built on this output is therefore verbatim duplication on every posting it
  appears for. It shipped, ran on prod for a day, and was removed; see
  `openspec/changes/surface-job-requirements/tasks.md` §10.
  - The value of this column is structure, not display: a `{text, priority}` list a
    matcher or an API consumer can read, extracted from prose no consumer can.
  - If a surface for it is ever wanted, the honest one shows what the reader cannot
    already see — the MODEL's list, which condenses prose into items the description
    never presents as a list. That is `enrichment_version > 0` and a payload of its
    own, not this column.

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

## Measuring coverage

**Sample with `TABLESAMPLE`, never with `ORDER BY id LIMIT` or the API's default order.**
This number was estimated three times before the backfill finished and was wrong twice,
both times for the same reason: id order and recency both correlate with the SOURCE, and
the source is exactly what decides whether a posting states its requirements as a list.

| Method | Answer | Why it lied |
| --- | --- | --- |
| 164 postings from the API's default order | 12.8% | newest first, which skews to flat-text aggregators |
| 300k rows by `ORDER BY id` | 43.5% | oldest first, which skews to ATS boards |
| `TABLESAMPLE SYSTEM (5)` over 350k open rows | **28.0%** | — |

Measured after the full backfill, 2026-09-05: **28.0%** of open postings carry a
derived list. Together with the model's own 1.9%, **29.3%** of open postings show a
reader something — and the two barely overlap (6,806 + 98,410 entries in the sample
union to 102,707), which is the whole bet of this package holding: the model reads the
prose postings, the parser reads the marked-up ones.

A regex sweep for a requirements-shaped heading said 23% before any of this was built.
The shipped extractor is stricter than a regex on purpose: a missed section is a blank
space, while a benefits list under a "Requirements" heading is a false claim the reader
cannot detect.

## Limitations

- **Latin-alphabet headings, mostly English.** `normalizeHeading` transliterates before
  matching (the same fold `normalize.Slug` applies), so an accented heading is spelled
  once in the vocabulary — `Előnyt jelent` is entered as `elonyt jelent`, not as the
  `el nyt jelent` a plain ASCII filter would have produced. Hungarian is in; more
  languages are an additive change to the three vocabularies.
- **No clustering.** Near-duplicate phrasings ("excellent written and verbal
  communication skills" vs "strong written and verbal communication skills") are stored
  as stated. Real, but a different problem.
