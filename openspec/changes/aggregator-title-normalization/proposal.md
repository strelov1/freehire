## Why

Cross-source aggregator suppression pairs an aggregator posting with its first-party ATS twin by
company + normalized title + compatible country. The normalization already drops a trailing
`— …` / `| …` clause. It does not drop a trailing `: …` clause, so an aggregator that decorates a
title that way never finds its twin and both cards stay in the catalogue — a shape `whatjobs`
produces constantly:

```
whatjobs «Senior Software Engineer: Full-Stack with TypeScript»  ↔  greenhouse «Senior Software Engineer»
whatjobs «Senior Lead Software Engineer: Golang, Kubernetes…»    ↔  workday «Senior Lead Software Engineer»
```

Measured on prod across the 784 companies present in both `whatjobs` and a real ATS (2406 whatjobs
rows):

| normalization | rows finding an ATS twin |
|---|---|
| trailing `—`/`\|` clause (today) | 520 |
| + trailing `: …` clause | **536** (+16) |
| + parenthetical group | 575 (+55) — **rejected, see below** |

## What Changes

- The aggregator-suppression title key additionally ignores a **trailing `: …` clause**.

That is the whole change. Two other decorations were measured and **deliberately rejected**, because
each one merges genuinely different roles:

- **Parenthetical.** Dropping it accounts for 39 of the 55 extra matches, and they are wrong:
  `«Senior Software Engineer, Backend (Traffic)»` would merge with `(Payments)`, `(Identity)`,
  `(Infrastructure)`, `(ML Feature)`, `(Lake Analytics)` — separate teams at one company, not
  reposts of one job.
- **Comma.** `«Senior Software Engineer, Backend»` would merge with `«…, Fullstack»`.

Both carry meaning; only the colon clause is reliably decorative in this corpus.

## Capabilities

### Modified Capabilities

- `aggregator-ats-dedup`: the decorated-title match key additionally ignores a trailing colon clause,
  and the spec records that parenthetical and comma clauses are meaning-bearing and must not be
  stripped.

## Impact

- **Code:** the `ntitle2` expression in `SuppressAggregatorDuplicatesForCompany`
  (`internal/db/queries/jobs.sql`) — extended in place, so the query keeps its two hash-join match
  paths and does not go quadratic (the existing comment warns about exactly that).
- **Data:** ~16 more rows gain `duplicate_of` per reindex on today's catalogue. Reversible: the pass
  re-evaluates from scratch each run and a row with no twin resolves back to NULL.
- **Honest scope:** this is a 3% improvement on the aggregator pass, not a fix for aggregator
  duplicates in general. Titles that differ by more than a trailing clause — the `whatjobs`
  "Senior Golang Engineer for Distributed Cloud & CI/CD" class — need a similarity signal, which is
  what `fuzzy-description-role-dedup` addresses next.
