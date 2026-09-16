## 1. Write the delta

- [x] 1.1 MODIFY `Frontend client and server error capture`: replace unconditional capture
      with the defect/condition rule, and state that an unreadable failure is still
      reported. Full requirement text, not a fragment — a partial MODIFIED block loses the
      rest at archive time.
- [x] 1.2 MODIFY `Backend panic and unexpected-error capture`: replace the three-item
      enumeration with the rule it was an instance of, naming `classify()` as the single
      decision point, and add the streamed-refusal scenario that `reportStreamFault`
      already satisfies.
- [x] 1.3 Confirm both `### Requirement:` headers match the live spec exactly, or the
      delta will not apply to the right requirement.

## 2. Check the delta against what actually ships

No code changes here — these tasks verify the words match the behaviour, which is the only
way this change can be wrong.

- [x] 2.1 Read `web/src/lib/sentryNoise.ts` and confirm every condition the new wording
      admits is one the filter actually drops, and that nothing the filter drops is
      unmentioned.
- [x] 2.2 Read `classify()` in `internal/api/handler/errors.go` and confirm every instance
      the new wording lists is a real case there.
- [x] 2.3 Confirm the existing tests already pin this behaviour, so the requirement is not
      describing something unverified: `web/src/lib/sentryNoise.test.ts` and
      `internal/api/handler/errors_test.go`.
- [x] 2.4 Run the suites unchanged and confirm nothing moves — a spec-only change that
      alters a test result is not a spec-only change.

## 3. Close out

- [x] 3.1 PR, CI, merge. (#2915)
- [x] 3.2 `/opsx:archive` then `/opsx:sync`, which is what writes these requirements into
      `openspec/specs/error-tracking/spec.md`. Until that runs, the live spec still
      contradicts production — the archive IS the fix, not a formality.
