## Context

See proposal.md — Why. Today contacts live only inside stamp-gated `resume_structured`; Profile is read-only; pending strips semantics; tailor seed can open without summary/skills/projects. Experience API can update employments; the SPA cannot edit project metadata.

## Goals / Non-Goals

**Goals:**

- Durable owned contacts as the seed/Profile source of truth for identity/links.
- Visible parse status + retry without re-upload.
- Tailor/base seed retains summary, skills, projects under pending stamp.
- Experience UI edits for project rows.

**Non-Goals:**

- Editable summary/education/skills as free-form Profile fields in v1 (display from current extract only).
- Softening stamp equality for “current structure.”
- Auto-overwriting hand-edited contacts on every extract.
- Changing agent `cv_edit` contact policy (agent still cannot patch contacts).

## Decisions

1. **Separate storage for candidate contacts** (users columns or small jsonb), not writing into `resume_structured`. Keeps extract honest and deletes independent.
2. **Fill-empty on extract; explicit “replace from CV” for overwrite.** Matches the earlier open question; avoids wrong-link ping-pong.
3. **Pending body for seed only:** when stamp mismatches, seed may copy summary/skills/education/languages/projects/certs from the last blob into the CV document. Profile still does not claim that blob is current. Trade-off: old-file semantics may briefly appear on a new base — better than blank; user can edit CV or wait for extract.
4. **Parse status:** persist last attempt outcome for the current `resume_uploaded_at` (pending default, ok on success, failed + short code/reason on error). Retry calls the same extract path as upload background work.
5. **Projects:** reuse `PUT/POST /me/experience/employments`; wire SPA forms only.

## Risks / Trade-offs

- [Pending seed uses old summary] → Mitigation: parse-status banner on Profile and tailor; retry; user edits CV body.
- [Contacts/jsonb migration] → Mitigation: backfill owned contacts from current or provisional structure once on migrate/first read.
- [Failed vs pending ambiguity if we only log] → Mitigation: record failure against upload stamp; clear on success/retry start.

## Migration Plan

1. Add columns + backfill contacts from existing structure/provisional where owned empty.
2. Deploy API (contacts CRUD, status, retry) before SPA.
3. Ship Profile + Experience UI.
4. Adjust `bankedSeeder` / stale-base seed paths; follow with backfill-resume-structured for stuck users.

## Open Questions

- None blocking: “replace from CV” control can be Settings-only in v1.
