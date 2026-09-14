## Context

Confirmed live against `jobs.humanbit.ai/scrabble-jigsaw`: the listing page's RSC flight
carries `["$","$L19",null,{"jobBoard":"scrabble-jigsaw","jobs":[{"id":...,"title":...,
"status":"published","location":...,"salary_min":...,"salary_max":...,"salary_currency":...,
"description":"$<id>","org_name":"Scrabble & Jigsaw",...}]}]` — enough to enumerate every
posting and name the company, but with NO `employment_type`/`skills`/`remote`/`work_mode`.
A posting's own detail page (`.../jobs/<uuid>`) carries the FULL object under
`"job":{"id":...}` including those fields, confirmed on two live postings — but never
`org_name` (company comes only from the listing or the configured entry).

The listing page is ~5.6 MB (2000+ flight push chunks, almost entirely Next.js framework
code rather than job data) against a ~59 KB detail page — measured on the one live tenant
sampled (10 open postings). Fetching the listing once and hydrating each posting's detail
individually is lighter in total than trying to force everything out of the listing alone
would be if the listing scaled with framework weight rather than job count (unconfirmed
either way, but the asymmetry is stark enough not to bet the design on the listing staying
small).

`work_mode`, `payment_type`, `payment_on`, and `payment_currency` were all null on every
sampled posting; `remote` (a plain boolean) was populated (`false` on both samples).

## Goals / Non-Goals

**Goals:**
- Crawl a HumanBit tenant's open postings via one listing fetch (enumeration + company
  name) plus one detail fetch per posting (structured fields), following the
  `hiringthing`/`topco` "cheap list, per-item hydrate" shape.

**Non-Goals:**
- No `work_mode` string-enum mapping. The field exists in the detail schema but was null on
  every sampled posting, so there is no confirmed example of its real spelling to map from
  — mapping it would be a guess, not a reading. `Remote` (a confirmed boolean) drives the
  work-mode signal instead: `true` → `"remote"`, `false` → left for the location heuristic,
  the same "structured signal only, never invented" bar `scalis-source`'s salary_period
  non-goal already applied.
- No `SalaryPeriod`/`SalaryMin`/`SalaryMax` mapping. `payment_type`/`payment_on` (the
  fields that would state the unit — annual vs. monthly vs. hourly) were null on every
  sampled posting despite real `salary_min`/`salary_max`/`salary_currency` values being
  present. This codebase's own convention (`hiringthing.salary()`) is to report all bounds
  together with their period or none at all; reporting a number with no unit would be
  worse than reporting nothing.
- No tenant-discovery/harvest prober — only one live tenant (`scrabble-jigsaw`) is known
  today, the same reasoning `selfrecruit-source`/`scalis-source` already gave.

## Decisions

- **Skills are canonicalized through `skilltag.Parse`, not passed through raw.** Live
  entries are compound phrases ("Cost Accounting", "Zero-Based Budgeting"), the same shape
  `micro1Skills` already documents needing mining rather than whole-string matching. An
  earlier draft of this adapter assigned `j.Skills` verbatim, bypassing the dictionary
  entirely — caught in review before merge, since it would have shipped raw, uncontrolled
  strings into the `jobs.skills` facet, against `jobderive.go`'s documented "already
  canonical" precondition for a structured-source `Skills` list.
- **`fullBoardListing` still applies.** The listing proves the whole set of open postings;
  a detail-fetch failure for one posting becomes an Unreadable marker (never a silent
  drop), the same contract `successfactors`/`careerspage` already give a listing-then-
  detail adapter. A listing fetch/decode failure fails the whole `Fetch` outright.
- **Company name comes from the listing's `org_name`, not the detail page**, since the
  detail object never carries it; `firstNonEmpty(org_name, e.Company)` is evaluated once
  per listing fetch and threaded into every detail's Job, mirroring how `deel` reads
  `careerPageSettings.preferredOrganizationName` once and reuses it per posting.
- **Description is resolved from the DETAIL page's own flight rows, not the listing's.**
  Verified live: the detail page's `"description":"$19"` resolves against that SAME page's
  `nextFlightTextRows` (5908 chars of real HTML) — row `19` is simply absent from the
  listing page's flight. Each page's `"$<id>"` numbering is local to that page's own render,
  the same "resolve from the flight you just decoded" pattern every other RSC-flight
  adapter (`deel`, `micro1`, `topco`) already follows; an earlier draft of this design
  assumed the listing's rows could serve both, which live verification disproved before
  any code was written on that assumption.

## Risks / Trade-offs

- [Only one tenant confirmed, with no non-null `work_mode`/salary-period example] → a
  future tenant might expose these fields; the Non-Goals above are deliberately revisitable
  once a real example exists, not permanent exclusions.
- [The listing's ~5.6 MB weight is a real per-crawl cost] → bounded to once per crawl cycle
  (not once per posting), and no smaller enumeration endpoint was found live; accepted as
  the cost of a tenant this small platform's own app bundle carries.

## Migration Plan

No data migration. Ship the adapter, merge, deploy, then add `humanbit/scrabble-jigsaw` by
hand via `cmd/add-board` to close `board_submissions` id 177.
