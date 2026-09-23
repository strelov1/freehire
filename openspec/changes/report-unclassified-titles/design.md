## Context

`dictgap` holds two pure functions over rows the caller reads, and two thin binaries
that do the reading and the printing. Both mine LLM enrichment output. This adds a
third of the same shape, over a population enrichment never sees.

The shape is worth copying exactly rather than improving: `ListTitlesForClassifyDrift`
already solved the two hard parts. It groups PER CHUNK rather than across the table,
because an aggregate has no single id to resume from, and it carries no row `LIMIT`,
because `GROUP BY` already bounds the output at the number of distinct titles in the
range while a `LIMIT` on an unordered aggregate would silently drop titles. The caller
merges counts by title across chunks.

## Goals / Non-Goals

**Goals:**

- Make the measurement that found 71,314 lost postings repeatable by anyone.
- Answer for the dictionary as it stands today, not as a stored column remembers it.

**Non-Goals:**

- Writing anything. No `--apply`, no dictionary edit, no database write. A curator
  reads and decides, which is what the existing requirement already binds both reports
  to and what this one joins.
- A timer. Same reason: the output is a list for a person.
- Judging whether an unplaced title SHOULD be placed. The report ranks by volume; the
  corpus of "Музыкальный руководитель" and "Швея" sits in it and belongs there, because
  what makes a title worth a term is a judgement only a curator can make.
- Suggesting terms. A generated suggestion would be read as an answer, and the whole
  argument for a curated dictionary is that every entry was looked at.

## Decisions

**Recompute, never read `jobs.is_tech`.** The stored column is the OLD dictionary's
answer until `cmd/backfill-derive` reaches the row, and that pass runs at ~171 rows/s
over 12.7M rows — so for most of a day after a dictionary ships, the column and the
dictionary disagree. A report reading it would rank gaps already closed and hide gaps
the new terms opened, precisely when a curator is most likely to look. `dictgap`'s
drift report already makes this choice and says so in its doc comment; this change
promotes it to a requirement so both are held to it.

**BOTH dictionaries must fail.** A title placed by `classify.Parse` into a technical
category already yields `is_tech` through the derivation, so reporting it as a gap
would be wrong. The test is the same one `jobderive.deriveIsTech` applies, minus the
structured source hint, which is a fact about a crawl rather than about the title.

**Scoped to open, canonical, non-private.** The drift report deliberately scopes to
enriched postings regardless of state, because a title's dictionary answer is a fact
about the text. Here the question is different — "what is the catalogue failing to
publish" — so a title carried only by closed or duplicate postings is not a gap. This
is a deliberate divergence from the sibling query, and the spec states it.

**No resume knob, unlike the drift report.** Copying it would have been wrong here.
The ranking is over counts accumulated across the WHOLE id span, so starting at a
later id leaves the accumulator empty for everything before it: a title carried on
both sides of the cursor is undercounted, and the top-N describes a suffix of the
catalogue while looking exactly like a report on all of it. A read-only report can
simply be run again; a plausible wrong number cannot be spotted. A cancelled run
prints no partial report and says to start over.

**A new binary, not a flag on an existing one.** `report-classify-drift` reads enriched
postings and compares two opinions; this reads unenriched ones and compares against
silence. Sharing a binary would mean one flag deciding which query runs, which set is
scoped how, and which of two unrelated outputs is printed.

## Risks / Trade-offs

- **The report is long and mostly noise** → by construction: 1.56M distinct titles, of
  which perhaps 3% are gaps worth closing. Ranking by posting count is what makes it
  usable, and the measured shape supports it — the top of the list is dominated by
  seamstresses and retail apprenticeships, and the software titles show up in volume
  once the obvious non-tech mass is read past. A curator reads with a filter; the
  report does not pretend to do the judging.
- **Grouping per chunk undercounts nothing but splits rows** → a title spanning two id
  ranges returns twice and the caller sums, exactly as the drift report does.
- **A full scan over the catalogue is expensive** → same cost profile as the existing
  reports, hand-run and not scheduled, and it reads no `description` column.

## Migration Plan

Deploy is a new binary nothing calls. There is no rollout step and no rollback beyond
deleting it, because it writes nothing.

## Open Questions

None.
