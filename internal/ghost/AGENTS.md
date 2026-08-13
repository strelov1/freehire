# Ghost-signal conventions

## Scope
The hedged "is this posting real" verdict — `none` / `possible` / `likely` — derived from
two tiers of evidence, plus the cross-check that stamps aggregator postings absent from
the employer's own board. Pure classifier over scalars: no database, no clock of its own.
The verdict is TIME-DEPENDENT, so it is computed on read, never stored.

## Always true
- **Two tiers of evidence, and the tiers are the point** (classify.go:1-13). STRUCTURAL
  criteria describe the shape of a posting — `evergreen_posting` (from
  `internal/jobreality`'s class) and `ats_absent` (the cross-check stamp). OUTCOME criteria
  describe what happened to people — `silent_applications` and `user_reports`. Codes and
  their fixed order: classify.go:34-41, with `CriterionCodes` the same vocabulary as an
  ordered value. `CriteriaTotal` is `len(CriterionCodes)` — the served denominator
  ("2 of 4"), counting criteria that had no data so a reader sees how much is unknown.
  It is derived rather than written down because a hand-kept 4 beside a list of five is a
  scale that lies to every reader.
- **The web checklist is generated from the vocabulary, not synced to it.**
  `cmd/gen-contracts` emits `CriterionCodes` as `GHOST_CRITERION_VALUES`, and
  `web/src/lib/ghost.ts` keys its per-criterion wording off the generated union. The job
  page draws one gauge segment per criterion the payload says fired and accounts for each
  in the expanded list, so a criterion added here with no wording there would colour a
  segment above a checklist that could not explain it. Adding a criterion is therefore a
  compile error in the SPA until it is worded (`ghost.ts`) and illustrated
  (`SignalDiagram.svelte`), and a test failure until the landing explains it
  (`ghostSignals.ts`). Run `make gen-contracts` after touching the codes.
- **Structural evidence can NEVER produce `LevelLikely`.** `Classify` (classify.go:110) has
  two gates that are not the same gate: `convergence = 2` criteria must fire to say anything
  at all, and `ContributorGate = 2` DISTINCT people must have contributed outcome evidence
  for the stronger claim (classify.go:125-134). Enough witnesses reach `possible` even as a
  lone criterion — the outcome tier is the only one that observes reality, so it must be
  able to mark a posting by itself.
- **`ContributorGate = 2` is one number with two consequences** (classify.go:48-59). The
  classifier requires it, and the serving layer withholds the contributor count below the
  same threshold (`internal/jobview/ghost_classify.go:52`). A served count of one would
  deanonymise the single applicant to the employer, and one account must not be able to
  mark an honest posting alone — lowering it breaks a privacy guarantee and an abuse
  guarantee at once. Contributors are distinct people across BOTH channels: a person who
  applied and also reported is one witness (evidence.go:34-42).
- **An empty board-title list means "no coverage", not "absent"** (crosscheck.go:28-33).
  `Crosscheck` (crosscheck.go:37) judges one company's postings against the titles we crawl
  from that company's own `ats`/`company` boards; an empty list skips every posting and
  reports the skip. Stamping there would report our coverage gaps as the employer's fault —
  how the previous attempt at this feature failed. `Skipped` is always reported: a run that
  silently judged nothing looks identical to one that judged everything present.
- **Stamps expire and are re-stamped.** The reader ignores an absence stamp older than
  `absenceStampMaxAgeDays = 14` (classify.go:61-66), so a stopped worker falls silent rather
  than accusing the catalogue from a frozen snapshot; the worker therefore re-stamps a still-
  absent posting on every run instead of leaving the stamp alone (crosscheck.go:34-36).
  A posting already correct in the database produces no write.
- **Outcome judgement reuses `internal/userjob`'s silence ladder** (evidence.go:49-52,
  68-77). An application counts as silent only when `userjob.SilenceStateFor` returns
  outright `silent`; a user report counts only after the reported apply date clears the
  `applied` stage's own threshold. Restating thresholds here would let the same application
  be judged by two ladders with nothing binding them.
- **A closed job carries no signal** (`internal/jobview/ghost_classify.go:29-31`) — the
  posting is already down, so there is nobody left to warn.
- Title matching in the cross-check goes through `jobhash.RoleKey("", title)` — title alone,
  scoped by the fixed company. A title that yields no key is skipped, never stamped
  (crosscheck.go:53-61).

## Consumers
- `cmd/gen-contracts` — emits `CriterionCodes` into the web contracts as
  `GHOST_CRITERION_VALUES`; the SPA's checklist is keyed off it.
- `cmd/ghost-crosscheck` (main.go + report.go) — the worker that runs `Crosscheck` per
  company and applies the stamp/clear writes.
- `internal/handler/ghost_evidence.go` — gathers per-job outcome evidence in two batched
  queries for a page of jobs; fail-soft (a lookup failure downgrades the signal rather than
  failing the read).
- `internal/handler/jobs.go` — the job read path that attaches the signal.
- `internal/jobview/ghost_classify.go` — `ClassifyGhost`, the serving projection: omits the
  field at level `none`, withholds counts below the gate, exposes `ATSCheckedAt` only when
  the criterion fired.
