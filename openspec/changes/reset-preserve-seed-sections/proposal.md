## Why

Reset-from-résumé can leave a tailored CV without skills or summary even when the
candidate already had them on the page, or when a current structured extract carries
them. During a pending/stale extract the seed is identity-only for those sections while
the experience bank still supplies roles, so reset proceeds and whole-document replace
blanks skills and summary. That feels like data loss. Separately, a current extract that
does include skills/summary must land them after reset — that path needs an explicit
regression so it cannot silently regress.

## What Changes

- On reset-from-résumé (tailored copy and base refresh), when the seed has **empty**
  summary or skills, **keep** the document's existing summary/skills instead of blanking
  them (same keep-if-empty idea as header contacts).
- When the seed has **non-empty** summary or skills (current structured extract), those
  values **replace** the document's — seed wins; no stale-blob leak.
- **Do not** feed superseded structured summary/skills into the seed while the stamp is
  pending (existing provisional rule stays).
- Add regression coverage: (a) provisional + bank experience preserves prior skills/summary
  on the tailored CV; (b) current extract with skills/summary writes them after reset.
- Update the provisional-reset expectation that today asserts summary must be empty after
  reset when experience came from the bank alone.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `cv-tailoring`: reset-from-résumé must not blank skills/summary when the seed omits them;
  must apply them when the current extract provides them. Superseded structured summary/skills
  still MUST NOT enter the seed while the stamp is pending.

## Impact

- `internal/handler` — `applySeedContent` / reset path (and shared base reseed).
- Integration tests in `cv_reset_integration_test.go` (including the provisional+bank case).
- No API shape change; no change to first-time tailor mint beyond shared apply helper if used.
- No change to `jd-surface-align` preferred-surface rewrite (still runs on the tailored seed
  body after merge).
