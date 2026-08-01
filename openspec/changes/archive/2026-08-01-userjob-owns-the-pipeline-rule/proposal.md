## Why

"How an application may move through its stages" is a tracking-domain rule, and it was decided by
the mail-classification package. `internal/mailclassify` held `stageOrder` — a second, independent
encoding of the pipeline rank — and `terminalStages`, the settled-outcome set. So the mail
vocabulary and the application state machine were welded together, and `internal/userjob`, which
documents `Stages` as "the single source of truth", did not own the rule that gives those stages
meaning.

The one binding test between the packages ran in one direction only: every stage mailclassify
names must be a real stage. Nothing checked the reverse, and that gap admits a real drift —
insert `take_home` between `screening` and `interview` in `userjob.Stages` and it gets rank 0,
which ranks **below `applied`** and is not terminal, so any forward signal advances the
application **backward out of it**. `silence.go` and `buckets.go` go stale the same way; both
hand-list the same five active stages.

## What Changes

- `userjob.IsTerminal` and `userjob.Forward(current, target)` own rank and terminality, beside
  `Stages` and `silenceThresholds` which key on the same five active stages.
- `mailclassify.AdvanceStage` becomes signal→stage plus `userjob.Forward`. `signalStage` stays —
  what an employer's email *means* is genuinely mail's question. `stageOrder` and mailclassify's
  `terminalStages` are deleted.
- Rank stays an explicit table rather than the index in `Stages`: `accepted`/`rejected`/
  `withdrawn` sit after `offer` in the vocabulary but are outcomes, not positions.
- **Three guards, each verified to fire** by inserting the exact stage the finding names:
  - every stage in `Stages` is ranked **or** terminal, never both and never neither;
  - `silenceThresholds` covers exactly the active stages;
  - exactly one stage falls to `Aggregate`'s default bucket (`applied` → `no_answer`), so a stage
    added without a case shows up as a second one landing there.
- The cross-package test keeps its direction and gains one assertion: no signal may map to a
  terminal stage — deciding an application is settled is never an inference from mail.

No behaviour change. Every branch was compared: the old code tested terminality before looking up
the signal and the new one after, and both return `("", false)` either way.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

(none) — no requirement-level behaviour change. `tasks.md` is the real artifact; the change
archives with `--skip-specs`.

## Impact

- `internal/userjob` — a new `pipeline.go` and its tests.
- `internal/mailclassify/classification.go` and its test.
- `internal/inbox` and `internal/maillink` are unaffected: both call `AdvanceStage`, whose
  signature and answers are unchanged.
