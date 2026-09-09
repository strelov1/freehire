## Context

See proposal.md for motivation. This is the remediation half of #2710: that PR
fixed the detector; this fixes the catalogue's existing wrong rows.

## Decisions

**Recompute without the structured-signal input, on purpose.** `jobderive`'s
precedence is structured ATS → location marker → description phrase, each layer
only filling what the one above left empty. This pass re-implements only the
bottom two layers (`location.Parse(...).WorkMode`, then
`location.WorkModeFromDescription` + `RemoteContradicted`) and never reads the
row's stored `work_mode` as an input — only as the thing being corrected. Feeding
the stored value back in, the way `cmd/backfill-derive` does, would reproduce
exactly the bug this pass exists to fix: the old detector's output and a genuine
ATS signal are the same shape once written, and `backfill-derive`'s job is
specifically to never touch that shape.

**Candidates from Meilisearch, not a Postgres `WHERE description ILIKE ...`.**
Same reasoning `cmd/backfill-clearance` gives: a predicate over `description`
de-TOASTs the column for every row it examines (millions of reads to find a few
hundred matches). The search index already holds the text. Recall does not need
to be precise — the recompute step decides, and a candidate the recompute declines
(still resolves `"remote"`) simply keeps its stored value, at the cost of one read.

**Write only when the recomputed value differs from `"remote"`, never
unconditionally.** The candidate filter already scopes to `work_mode = "remote"`,
but the search index can lag Postgres by however long the last search-drain cycle
took, so the code re-checks the row actually read (`r.WorkMode != "remote"` →
skip) before deciding whether to write.

**No structured-signal recovery, and no attempt at one.** The three-layer
precedence collapses into a single stored column at write time; there is no
separate column recording which layer produced today's value, and adding one
purely to make this one-off pass more precise would be scope creep this
proposal.md's "Known limitation" section argues against — the self-healing
backstop (the next ordinary crawl restores a genuinely-structured value) is judged
sufficient.

**No bundled reindex, matching `cmd/backfill-clearance`.** `work_mode` is not part
of `content_hash`, so an incremental `search-drain` push would never carry a
corrected row's facet to Meilisearch; a full `make reindex` is a separate,
deliberate step after this pass, run once rather than folded into every backfill
tool's own binary (which would risk two concurrent rebuilds if two such tools ran
close together).
