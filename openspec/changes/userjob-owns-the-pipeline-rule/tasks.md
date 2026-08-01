## 1. Move the rule to the vocabulary that owns it

- [x] 1.1 `userjob.activeRank` + `terminalStages`, with the doc saying why rank is not the index
      in `Stages` and why the keys match `silenceThresholds`.
- [x] 1.2 `userjob.IsTerminal` and `userjob.Forward(current, target)`. `Forward` refuses a target
      that is not an active stage, so advancing INTO a terminal outcome is impossible by
      construction rather than by the accident of terminals ranking 0.
- [x] 1.3 `mailclassify.AdvanceStage` becomes signal→stage plus `userjob.Forward`; delete
      `stageOrder` and mailclassify's `terminalStages`. Compare every branch for behaviour.

## 2. Close the direction the test never checked

- [x] 2.1 Every stage in `Stages` is ranked or terminal — exactly one of the two.
- [x] 2.2 `silenceThresholds` covers exactly the active stages, in both directions.
- [x] 2.3 Exactly one stage falls to `Aggregate`'s default bucket — the switch cannot be
      introspected, but its shape can.
- [x] 2.4 Prove all three fire rather than assuming: insert `take_home` into `Stages` — the stage
      the finding names — and confirm the failures name it.
- [x] 2.5 The cross-package test gains: no signal maps to a terminal stage.

## 3. Verify and close

- [x] 3.1 `go test ./...` AND `go test -tags=integration ./...`.
- [x] 3.2 Mark S16 ✅ in `docs/reviews/2026-08-01-architecture-review.md`.
