## Context

See proposal.md for motivation. Today the seed path is:

```
resume_structured (stamped)  +  experience bank (unstamped)
                │
                ▼
         bankedSeeder.Structured
                │  seedable(st) ← true on experience alone
                ▼
            cv.Seed(st)  →  Document.Header from structure contacts
                │
                ▼
   applySeedContent / Create / CreateTailored   (whole-document replace)
```

Contact fields on `Structured` are restored from `pii.Contacts` after extraction — except `location`, which is left for the LLM while the residence line is typically masked as `ADDRESS`. Separately, `seedable` and `applySeedContent` let a structure-absent composition overwrite a filled header.

Agent redaction (`GetForModel` / policy denials) is unrelated to this bug and stays as-is.

## Goals / Non-Goals

**Goals:**
- Location round-trips through detection → `Contacts` → `Structured` → `cv.Seed` → CV header / geography derivation.
- Structure-absent / bank-only compositions cannot create or replace a CV document.
- Applying a seed onto an existing CV cannot blank header fields the seed left empty.

**Non-Goals:**
- Backfilling already-blanked `cvs` rows, or rewriting historical `resume_structured` blobs.
- Changing what the tailoring agent may read or patch (contact strip + path policy stay).
- Broadening phone regex / ATS contact scoring (owned by `ats-contact-detection`).
- Moving location out of `Professional` / keeping it from models — location remains a fit signal; only the extraction restore path changes.

## Decisions

**D1 — Extend `pii.Contacts` with `Location`, filled from the first plausible `ADDRESS` span.**
Parallel to name/email/phone. `fillContact` records it; `resumeextract.Extract` assigns `s.Location = c.Location` (overwriting any model value, which should be empty/null after redaction). *Alternative:* ask the model for location and `Restore` placeholders in its output — rejected: contact fields were deliberately removed from the extraction schema so the model never invents them; restore-from-detection is the existing pattern. *Alternative:* stop masking `ADDRESS` — rejected: residence is PII and the detector's job.

**D2 — Plausible location is a non-empty trimmed address span, not a second name heuristic.**
Unlike `isPlausibleName`, address text is messy ("Lisbon, PT", "Remote — Berlin"). Accept the first non-empty `ADDRESS` the redactor already collected; do not invent a city parser here. A wrong first span is rare next to a missing location; if it bites, tighten later.

**D3 — Usable seed requires a current structured résumé (`ok` from `Structured`), not `seedable` on the composed value alone.**
`bankedSeeder.Structured` already returns `(st, seedable(st), nil)` where experience can make `seedable` true with a zero structure. Change the boolean to mean "safe to whole-document seed": current structure present **and** the composed value still has something to seed (reuse `seedable` on the composition only after `ok`). Callers (`Tailor`, `reseedBaseIfStaleVsUpload`, `ResetCVFromResume`) keep their existing 409 wording paths.

**D4 — `applySeedContent` merges header contacts field-by-field.**
For each of `full_name`, `email`, `phone`, `location`, `links`: if the seed value is empty (or links empty) and the keep value is not, keep the existing. Non-empty seed wins. Body sections still come entirely from the seed (experience from the bank is the point of reset). *Alternative:* skip the whole reseed when any contact is missing — rejected: a résumé that truly has no phone should still refresh experience. *Alternative:* only the usable-seed gate — rejected: defence in depth for a partial extract that omitted one field.

**D5 — Keep location in `Professional` and in `withoutContacts`.**
No change to model-facing projections. Location is fit geography, not an agent-denied identifier; restoring it into `Structured` simply makes those projections truthful again.

## Risks / Trade-offs

- [First `ADDRESS` span is an employer office] → Mitigation: accept for now; detector tags are `private_address`; add a fixture if a real CV mis-tags. Geography derivation already tolerates unresolved lines.
- [Users mid-extraction get 409 instead of a blank-header CV] → Intentional: better to wait than to wipe contacts. UI already knows extraction is background.
- [Reset refuses until structure lands] → Same; the bank alone was never enough to rebuild the header correctly.
- [Existing blanked tailored CVs stay blank until reset/re-tailor] → Document in migration; no silent rewrite.

## Migration Plan

1. Ship D1–D4 together (extract restore + seed gate + header merge). No schema change, no new env.
2. Candidates with blank headers: re-upload (to refresh structure with location) then reset-from-résumé, or edit the header by hand. New tailor bootstraps after a successful extract get a filled header.
3. Rollback: revert the code; no data migration to undo.

## Open Questions

None that block implementation. Whether a second `ADDRESS` should prefer a span near the name line can wait for a real mis-tag incident.
