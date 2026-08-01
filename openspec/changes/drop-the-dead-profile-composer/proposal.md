## Why

`experience.ProfessionalFrom` is an exported function with a sixteen-line doc comment defending
its design and seven dedicated tests — and **no production caller**. Verified by repo-wide grep:
the only references were its own declaration and its own tests.

It duplicates `Store.Professional`'s composition, so a change to how the candidate profile is
assembled had two places to land, and one of them was exercised only by tests that would keep
passing either way. `internal/resumeextract/AGENTS.md` — the file a reader consults first to
answer "what does the fit chain send?" — pointed at the dead one.

## What Changes

- `ProfessionalFrom` is deleted.
- **Its rationale is not.** The sixteen lines explaining why work history comes from the bank,
  why the structure's own experience is ignored rather than used as a fallback, and why contacts
  never cross over move onto `Store.Professional`, which is the path that actually runs them.
- Its seven tests move to `Store.Professional` rather than to `experienceFromBank`. That is
  strictly more coverage than they had: the assertions now run through the store reads the live
  path makes, not around them. Two small helpers keep them readable — atoms name their employment
  by index, because the ids are the store's to mint.
- `internal/resumeextract/AGENTS.md` names `experience.Store.Professional`, and records that it
  said otherwise until today, so the next reader knows the old name was not simply renamed.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

(none) — no requirement-level behaviour change. Nothing called the deleted function;
`tasks.md` is the real artifact and the change archives with `--skip-specs`.

## Impact

- `internal/experience/professional.go` (−22 lines of function, +14 of relocated rationale),
  `internal/experience/professional_test.go`, `internal/resumeextract/AGENTS.md`.
