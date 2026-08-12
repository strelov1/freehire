## Context

See proposal.md — Why. Reset today builds a seed via `StructureForSeed` + bank overlay,
then `applySeedContent` whole-document-replaces the body while only merging header
contacts keep-if-empty. Provisional/stale stamps strip summary/skills from the seed
(`provisionalContacts`) by design (`cv-provisional-header-seed`). Bank experience alone
makes `hasSeedBody` true, so reset runs and blanks those sections on the page.

A current extract that includes summary/skills already maps through `cv.Seed` into
`applySeedContent`; that path needs a hard regression test. Keep-if-empty for empty seed
sections covers pending extract and extracts that simply omit skills/summary without
resurrecting superseded blob text.

## Goals / Non-Goals

**Goals:**

- Keep prior tailored (and base) summary/skills when the seed leaves them empty.
- Ensure non-empty seed summary/skills always replace on reset.
- Preserve the provisional rule: superseded blob semantics never enter the seed.

**Non-Goals:**

- Feeding last-blob summary/skills into `StructureForSeed` while pending (explicitly
  rejected by `cv-provisional-header-seed`).
- Deriving CV skills from the experience bank or profile on reset.
- Changing first-time tailor mint beyond the shared apply helper.
- Extending keep-if-empty to education/languages/certs in this change (skills + summary
  only; note the seam if the same wipe shows up there).
- Surface-align behaviour (unchanged; still runs on the tailored seed after merge).

## Decisions

### D1 — Merge in `applySeedContent`, not in `StructureForSeed`

Keep-if-empty for summary and skills lives next to `mergeSeedHeader`: after taking the
seeded document as the body default, if `seeded.Summary == ""` keep `keep.Summary`; if
`len(seeded.Skills) == 0` keep `keep.Skills`.

**Why:** Seed composition stays honest (provisional = no semantic body). The CV row is
what the candidate sees; blanking it is the bug. Alternatives considered: refuse reset
until stamp is current (breaks provisional+bank flows that already ship); seed last-blob
semantics while pending (rejected earlier).

### D2 — Seed wins when non-empty

Non-empty seed summary/skills always replace. No three-way merge with "prefer longer".

### D3 — Same helper for tailored reset and base reseed

Both call `applySeedContent`, so both get the rule. Base must not receive JD-aligned
wording (existing align-only-on-tailored split stays).

### D4 — Update the provisional integration expectation

`TestResetCVFromResume_ProvisionalContactsPlusBankSucceeds` (or equivalent) today asserts
summary stays empty after reset. Change it to seed a tailored CV that already has
summary/skills and assert they survive; still assert the seed composition did not carry
superseded summary into `StructureForSeed`.

### D5 — Case 2 regression

Add an integration test: current stamp + extract with skills/summary → reset → document
carries them (and surface-align may rewrite chip wording on the tailored copy only).

## Risks / Trade-offs

- **[Risk] Candidate thinks reset "restored" résumé skills that were never re-extracted** →
  Mitigation: only keep what was already on the CV; UI copy can stay "from résumé" for
  experience; skills/summary persistence is continuity, not a claim that extract finished.
- **[Risk] Stale skills linger after a real résumé that legitimately dropped them** →
  Mitigation: once extract is current with an explicit empty skills list, seed skills are
  empty and keep-if-empty retains prior chips — same as today for an extract that omits
  skills. If product later needs "empty extract means clear skills", that is a separate
  change (would need a presence signal, not just emptiness).
- **[Risk] Education/languages still wipe under provisional** → Mitigation: out of scope;
  same pattern if reported.

## Migration Plan

Deploy with the handler change; no schema migration. Rollback: revert `applySeedContent`
merge; provisional tests flip back.

## Open Questions

None — skills + summary only was chosen from the reporter's cases; other sections deferred.
