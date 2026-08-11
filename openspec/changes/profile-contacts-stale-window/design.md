## Context

See proposal.md. Observed locally: `resume_uploaded_at` is newer than `resume_structured_uploaded_at`, bank has employments, and the superseded `resume_structured` JSON still holds `full_name` / `phone` / `links`. `GetResume` clears the stale structure, overlays bank `Experience`, and returns that shell — `ResumeStructuredView` then shows experience with no identity.

The open `experience-bank` delta intentionally served bank experience through the stale window with empty contacts; that matches the API but fails the Profile tab expectation.

## Goals / Non-Goals

**Goals:**

- Cookie `GET /me/resume` keeps bank experience through the stale window **and** restores provisional contacts from the superseded blob when present.
- Explicit pending/stale signal for the Profile tab copy.
- Contact-free `GET /me/profile` `cv` block / Professional projection unchanged.
- Diagnose why background extract did not stamp the latest upload (so the window ends).

**Non-Goals:**

- Serving superseded education/summary/skills as if current.
- Editing contacts on the profile page.
- Changing assistant/API-key contact stripping.

## Decisions

### 1. Provisional contacts from the superseded blob

**Choice:** When `Store.Structured` returns `ok=false` but the raw row still has a structured JSON blob, unmarshal contacts (and only contacts / location / links) for the cookie résumé read. Semantic sections stay empty until the stamp matches.

**Why:** Identity rarely changes on a same-person re-upload; blanking the header is worse than briefly showing yesterday’s phone. Education/skills from a superseded CV would be wrong more often.

**Alternatives:** Null `structured` whenever stale (breaks bank-through-stale Profile list). Serve full superseded structure (violates freshness for semantic fields). Synthesize contacts from `users.email` only (loses phone/links/name).

### 2. Pending signal on the résumé meta

**Choice:** Add a boolean (e.g. `structure_pending` or `structure_current: false`) on the existing résumé status payload next to `structured`, set when a résumé is present but the stamp does not match (or extract never landed).

**Why:** The UI cannot infer intent from “experience without name”.

### 3. Profile tab copy

**Choice:** When pending, show a short line that the latest CV is still being parsed; still render provisional contacts and banked experience when present.

### 4. Stuck extract

**Choice:** Separately verify upload → `extractStructuredResume` path (LLM/PII config, logs) and, if needed, add a visible failure/pending path so a multi-day stale stamp is actionable (re-upload or retry). Not a schema migration.

### 5. Implementation seam

**Choice:** Extend the résumé store with a read that returns `(current Structured, ok, provisionalContactsFromBlob, pending)` or have `GetResume` call a lower-level get that does not discard the blob when stamps differ. Keep `Store.Structured`’s `ok` semantics for seed/usable-structure gates.

## Risks / Trade-offs

- **[Risk] Provisional contacts from an old CV after someone else replaced the file** → Mitigation: window ends when extract stamps; pending banner; contacts-only not full structure.
- **[Risk] Diverges from experience-bank draft delta (“no contacts”)** → Mitigation: this change supersedes that scenario for the cookie résumé read; document in archive notes.
- **[Risk] Stuck extract leaves provisional forever** → Mitigation: task to inspect local/prod extract failures and surface pending clearly.

## Migration Plan

None for schema. Deploy API + web together so the pending flag is understood. Existing stale rows start showing provisional contacts immediately after deploy.

## Open Questions

None — provisional contacts + pending flag is fixed above.
