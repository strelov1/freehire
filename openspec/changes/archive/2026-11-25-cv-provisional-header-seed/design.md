## Context

See proposal.md for motivation. Today `Store.Structured` returns ok=false when the stamp is stale; `bankedSeeder` therefore marks the seed unusable even when `ProfileReadForUser` can still surface provisional contacts. Tailor copies `base.Document` as-is; empty-header base rows stay empty. `mergeSeedHeader` / `applySeedContent` already encode “empty seed must not blank keep.”

## Goals / Non-Goals

**Goals:**
- One shared provisional-contacts helper used by profile read, seed composition, and header heal.
- Usable seed in the pending window when provisional contacts + seedable body (usually bank) exist.
- Persist empty-header heal on tailored open (and base when empty) without full document replace.

**Non-Goals:**
- Auto-running full reset-from-résumé on every open once extract completes.
- Feeding superseded summary/education/skills into seed.
- Changing the agent redaction of contacts for the model.

## Decisions

### 1. Expose provisional contacts from `internal/resume`, keep stamp gate

**Choice:** Add a small API on the résumé store (e.g. `ProvisionalContacts(ctx, userID) (Structured, bool, error)` or reuse the contact slice of `ProfileReadForUser`) that returns identity fields from the blob when stamp ≠ upload. Leave `Structured` stamp-gated.

**Why:** Specs require the stamp gate to stay false; seed layers contacts on top. Profile already has `provisionalContacts` — share it rather than duplicate.

**Alternatives:** Flip `Structured` to return provisional full blobs (rejected — would reintroduce superseded semantic sections). Soften only in the handler with raw SQL (rejected — bypasses the store contract).

### 2. `bankedSeeder` layers provisional contacts when structure is absent

**Choice:** When `resume.Structured` is not ok, load provisional contacts into `st`’s contact fields only; keep experience/projects bank logic unchanged; `haveStructure` for usable becomes `currentStructure || provisionalContactsNonEmpty` combined with `seedable(st)` (name alone can satisfy seedable via existing `FullName != ""`).

**Why:** Matches “provisional + bank usable” without inventing a second seeder type.

**Alternatives:** Separate `ProvisionalSeeder` (unnecessary). Make bank-only usable again (rejected — blanks header when no contacts).

### 3. Heal on tailored load, not only on reset

**Choice:** On owner GET of a tailored CV (and on tailor bootstrap when returning an existing copy), if header contacts are all empty and provisional contacts exist, `mergeSeedHeader` + persist via the editor/commit path (or a thin store update that still goes through sanitization). Also heal empty base under the same condition so the next tailor copy is clean.

**Why:** Existing empty tailored rows never call reset; open is the user-visible moment.

**Alternatives:** Heal only inside `Tailor` (misses reopen of an already-minted copy via GET). Client-side fill (rejected — PDF/export would still be blank).

### 4. “Empty header” means all contact fields empty

**Choice:** Trigger full heal when name, email, phone, location, and links are all empty; for partial headers, still fill only empty fields when we heal (same merge). Spec scenario covers partial via merge rules; we can heal whenever any contact field is empty and provisional has a value — simpler and matches mergeSeedHeader. Prefer: run merge whenever provisional contacts exist and at least one header field is empty (avoid write if merge is a no-op).

**Why:** User’s case is fully blank; partial fill is cheap and correct under merge rules.

## Risks / Trade-offs

- **[Risk] Stale contacts after a real identity change on re-upload** → Mitigation: contacts are provisional until stamp matches; extract overwrite then reseed/reset brings current identity. Merge never overwrites non-empty typed fields.
- **[Risk] Extra write on every GET** → Mitigation: compare pre/post merge; skip commit when unchanged.
- **[Risk] Diverges from cv-seed-contact-blanking “current only” wording** → Mitigation: this change explicitly supersedes that gate for the provisional-contacts case; bank-only remains blocked.

## Migration Plan

Deploy code only. No schema change. Existing blank-header CVs heal on next tailored open when a superseded blob still has contacts. If extract never ran and no blob exists, behavior unchanged (still empty).

## Open Questions

None — heal-on-open vs reset-only resolved by the product decision (heal on open).
