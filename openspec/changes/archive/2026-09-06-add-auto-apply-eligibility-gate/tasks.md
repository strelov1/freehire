## 1. Enqueue-time eligibility gate

- [x] 1.1 Add a shadow/enforce flag (e.g. `AUTO_APPLY_ELIGIBILITY_ENFORCE`) read once at
      startup, mirroring the `PLAN_ENFORCE` pattern in `internal/ai/plan`
      (`autoApplyEligibilityEnforce`, a `sync.OnceValue`-memoized env read — a plain
      constructor-threaded flag was rejected to avoid rippling through the ~78
      integration-test call sites `newAssistantHandlers` has, per root AGENTS.md's own
      warning about that constructor)
- [x] 1.2 In `internal/api/handler/auto_apply_enqueue.go`'s `PostJobAutoApply`, call the
      existing `jobBlockers` helper for the (candidate, job) pair before enqueueing
      (via `h.cv.match.jobBlockers`, reusing the already-wired `*matchHandlers` reachable
      from `assistantHandlers` — no new field needed)
- [x] 1.3 Filter the returned `[]hardconstraint.Blocker` to `work-authorization` and
      `location-and-work-mode` entries with `met == false` (`firstEligibilityBlocker`,
      split out as a pure function for unit testing)
- [x] 1.4 When the flag is in shadow mode: log the would-be refusal (candidate, job,
      blocker category, reason) and enqueue as today
- [x] 1.5 When the flag is enforced: refuse to enqueue, returning a response that names
      the blocker's reason (not a generic error), and do not call `EnqueueAutoApply`
- [x] 1.6 Unit tests: unmet work-authorization blocker refuses (enforced) / logs only
      (shadow); unmet location-and-work-mode blocker refuses; no blocker or unevaluable
      category (missing evidence either side) proceeds unchanged in both modes
      (`auto_apply_eligibility_test.go`, against the pure `firstEligibilityBlocker`)

## 2. Form-resolution geography park rule

- [x] 2.1 In `internal/api/atsapply`, add a geography/residency label-term list separate
      from `sensitiveTerms` (`sensitive.go`) — seed it from real captured `apply_forms`
      question labels (resolves design.md's Open Question on the exact phrase list)
      (`geography.go`'s `geographyTerms`, seeded from the live Garner Health wording)
- [x] 2.2 Add the recognizer function (e.g. `isGeographyLabel`) alongside the existing
      `isSensitiveLabel`
- [x] 2.3 Wire the check into `resolve.go`/`draft.go`'s ordering: after id-match and
      label-match resolution fail for a required field, check `isGeographyLabel` before
      `draftable`/`ResolveWithDrafting` is invoked; on a match, mark the field parked with
      a named reason instead of drafting (`draftable` now also requires
      `!isGeographyLabel(f.Label)`, so it parks with `Resolve`'s existing "no known answer
      source" reason exactly as a sensitive field does — no separate reason string, for
      consistency with that established pattern)
- [x] 2.4 Unit tests: a required unmapped geography-labeled question parks and is never
      passed to the drafter; a geography-labeled question already resolvable from known
      answers (e.g. city) still fills normally; an unrelated required question is
      unaffected and still reaches drafting as before

## 3. Observability

- [x] 3.1 Add a metric or structured log line for each shadow/enforced refusal, tagged
      by blocker category, so the false-positive rate among Pro candidates can be read
      before flipping the flag (per design.md's Migration Plan) (a `log.Printf` in both
      branches, tagged "would refuse"/"refused" plus the blocker category — no metrics
      system wired into this handler package today, so a log line is the proportionate
      choice; a real metric can follow if the shadow log shows this is worth graphing)

## 4. Documentation

- [x] 4.1 Update `internal/application/autoapply/AGENTS.md` to document the enqueue-time
      eligibility gate, its shadow/enforce rollout, and that a parked attempt from this
      gate is not a retry candidate
- [x] 4.2 Update `internal/api/atsapply/AGENTS.md` to document the geography park rule
      and why it is separate from the sensitive-keyword gate
- [x] 4.3 Update `internal/candidate/hardconstraint`'s package doc (no AGENTS.md exists
      for this package) to note that this is a second, non-advisory caller of `Evaluate`,
      and that the package's own "never hides or downranks" invariant still holds for
      every other caller

## 5. Verification

- [x] 5.1 `go build ./...`, `go vet ./...`, `go test ./...` — all clean
- [x] 5.2 `go vet -tags=integration ./...` — clean, including the ~78
      `newAssistantHandlers`-calling integration test files this change deliberately did
      not touch the signature of; `go test -tags=integration ./internal/api/handler/...
      ./internal/api/atsapply/...` — both pass
- [x] 5.3 Replayed as a unit test rather than a live network call (deliberately — no
      reason to re-touch a real employer's form for this): `TestIsGeographyLabel_
      CatchesTheLiveGarnerHealthWording` and `TestResolveWithDrafting_
      NeverDraftsAGeographyField` use the exact live wording ("Current State of
      Residence") and confirm it now parks via `isGeographyLabel`, not via an accidental
      dropdown-option mismatch
