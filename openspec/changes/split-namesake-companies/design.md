## Context

Company identity today answers one question — "are these two spellings one employer?" —
and answers it from names, with `company_slug_aliases` recording the merges a fold cannot
reach. The opposite error has no home: two employers with one name. Its cost is larger than
a duplicate, because the merged company also collects the identifying facts of whichever
namesake a name-matched backfill found first, and every lookup by those facts
(`by-domain`, LinkedIn, logo) then lands on the wrong jobs.

Measured on the replica (2026-11-24): `boards` already labels the right employers
(`greenhouse/gomotive` → "GoMotive", `ashby/exa` → "Exa Labs"), and those boards' postings
sit under their own slugs. What is wrong is the name-matched enrichment on the short slug,
plus boards whose label is a bare word shared by several employers.

## Goals / Non-Goals

- Goal: a lookup by domain reaches the employer that owns the domain.
- Goal: an ATS board whose employer is known by domain is never merged into a namesake.
- Goal: a moderator can undo a namesake merge, reversibly, and it stays undone.
- Non-goal: splitting aggregator postings (no board domain to go on).
- Non-goal: an automatic split. Like merges, a split is a judgement: the report proposes,
  a person decides.

## Decisions

1. **Domain evidence lives on the board, not the company.** A board belongs to one
   employer by construction; a company slug is a name and may stand for several. Keeping
   the domain on `boards` means it survives the company row emptying out, the same reason
   `company_slug_aliases` is not derived from `jobs`.
2. **Precedence, not overwrite, for company_info.** Writers keep their fill-gap rule
   (`company-info` spec); they gain one refusal (a website contradicting the slug's own
   board domains) and one preference (the slug whose board names the domain). Nothing
   already stored is rewritten by this change; the report lists what to correct.
3. **The split re-keys a board, not postings.** The decision is "board B is employer X",
   recorded as a board → slug override read by the pipeline's canonical resolution. A
   per-posting move would be undone by the next crawl.
4. **The new slug is derived, then frozen.** Name plus the domain's leading label
   (`motive-wearemotive`); once elected it never moves, like a merge canon, because the URL
   has started being indexed.

## Risks / Trade-offs

- A parent group's domain can stand behind several boards (subsidiaries). The report
  shows those as candidates; the moderator leaves them merged. No automatic action.
- A board's stated website can be a careers microsite or an ATS-hosted page. The domain
  is reduced to the registrable host and ATS hosts are refused (`atsboard` already knows
  them).

## Migration Plan

1. Add `boards.company_domain` and the override table (additive).
2. Fill domains from the ATS payloads that state them; ship the report.
3. Apply the company-info precedence to writers and `by-domain`.
4. Ship the `different` decision; correct `motive`, `exa`, `parallel` through it.

Rollback: drop the override rows; the pipeline falls back to name resolution.
