## Context

Skills currently reach `userprofile.skills` (the search-filter/match-scoring list) through
exactly one path: a client-computed merge (`web/src/lib/profileSkills.ts` `withSkill`/
`withSkills`) sent as a full `PUT /me/profile`. `internal/userprofile.Repository.Upsert`
replaces the whole profile row — there is no partial-column update — so every write, client
or server, must start from the current profile and rebuild all four fields.

The experience bank (`internal/experience.Store`) has two, and only two, atom-write entry
points regardless of caller: `AddAtom` (used directly by the chat tool `experience_add`, and
internally by `Import` for CV-upload atoms) and `UpdateAtom` (used by `experience_update`).
Both already sanitize and validate before persisting. This is the one place to add
bank→profile sync without touching every caller.

A profile is optional and specialization-gated: `search-profiles` requires a non-empty
`specializations` set for a profile to exist at all, and the current button this change
removes deliberately did nothing when there was no profile ("do not invent one" —
`web/src/lib/tailor/skillDiff.ts`). The sync must preserve that: it is a courtesy update to
an existing profile, never profile creation.

## Goals / Non-Goals

**Goals:**
- Every atom write (create or update, from any caller) keeps `userprofile.skills` current
  with the bank's canonical skills, without any explicit action from the user or the caller.
- Preserve the existing merge semantics exactly: case-insensitive de-dup, skip anything in
  `excluded_skills`, cap at `PROFILE_MAX_SKILLS` (200).
- Keep `internal/experience` and `internal/userprofile` decoupled at the concrete-type level
  (interface boundary, not a direct import of the other's full API), matching the existing
  `cvedit.EvidenceGate` pattern.

**Non-Goals:**
- No outbox/reconciler. Skill sync is additive and idempotent — a missed or failed sync
  self-heals on the next atom write for that skill, or the next time any atom naming it is
  touched. The staleness window this leaves is accepted (see Risks).
- No profile creation. A user with no saved profile gets no side effect.
- No change to `excluded_skills`, the job-match "I have it"/"avoid" claim flow
  (`job-profile-match`), or any public API shape.
- No retrofit of historical atoms. Only atoms written after this change ships trigger a
  sync; the bank's existing skills are not backfilled into profiles as part of this change
  (see Migration Plan).

## Decisions

**Interface boundary: `internal/experience` defines the port, `internal/userprofile`
implements it.** `Store` gains an optional dependency:

```go
// ProfileSkills is the narrow slice of userprofile the bank needs: fold newly banked
// skills into an existing profile. A user with no profile is not an error — there is
// nothing to fold into.
type ProfileSkills interface {
    MergeSkills(ctx context.Context, userID int64, skills []string) error
}
```

`internal/userprofile.Service` gets a new `MergeSkills` method implementing this — a
Get-modify-Upsert cycle reusing the same merge rule the frontend's `withSkills` already
encodes (server-side Go implementation, since this call has no client round-trip to lean
on). `Store` gets a `SetProfileSkills` setter (not a `NewStore` parameter, so every
existing construction site keeps compiling unchanged).

**Correction from the original plan above ("wiring happens once, at cmd/server"):**
`internal/handler` builds more than one write-path `Store`, not one. `SetProfileSkills` is
called at both: the shared `bank` in `handler.go` (backs the chat tools
`experience_add`/`experience_update`) and the separate `Store` `resume.go`'s
`newExperienceBank` builds for the CV-upload import path. `cmd/backfill-experience`'s bulk
historical-CV importer is deliberately left unwired — wiring it would contradict the "no
retrofit of historical atoms" non-goal below by flooding profile upserts for every
already-banked user on a rerun. The read-only `Store` instances in `cv_seed.go` and
`match_analysis.go` (`WorkHistory`/`Professional` only) never call `AddAtom`/`UpdateAtom`
and need no wiring.

*Alternative considered:* `internal/experience` importing `internal/userprofile` directly.
Rejected — no import-cycle risk today, but it is the shape this codebase has already
decided against once (`cvedit.EvidenceGate`), for the same reason: the bank should not need
to know the profile's full surface (specializations, location preferences, CV projection)
to fold in a skill list.

**Where it hooks: `Store.AddAtom` and `Store.UpdateAtom`, after a successful persist.**
Both are already the sole entry points for every atom-write caller (`Import` calls
`AddAtom` internally, so CV upload, `experience_add`, and any future manual-entry surface
all flow through one place). The sync call is best-effort: on error, log and return the
atom result as if sync had succeeded — the atom is already durably banked, which is the
more important fact, and a failed sync heals itself on the next write naming that skill.

*Alternative considered:* trigger sync from the assistant tool handlers
(`experienceAddTool`/`experienceUpdateTool`) instead of the Store. Rejected — `Import`
(CV upload) does not go through those handlers, so this would miss the CV-upload path
entirely, reintroducing exactly the kind of scattered-writer problem this change removes.

**Merge is additive-only, not a set-replace.** `MergeSkills` adds the given skills to
whatever the profile currently holds (mirroring `withSkills`); it never removes a skill the
bank no longer mentions (e.g., after an atom edit drops a skill tag). Symmetric removal
would require knowing whether any *other* atom still justifies that skill, which needs a
per-skill reference count this change does not build — deferred, not silently dropped (see
Open Questions).

**No new schema, no new column.** `userprofile.skills` is unchanged in shape; only a new
writer is added.

## Risks / Trade-offs

- **[Risk]** A profile-sync failure (transient DB error) leaves the profile stale for that
  one skill until it is re-synced. → **Mitigation:** low severity — search filters and
  match scoring already tolerate a profile lagging the bank (that gap is the entire reason
  this change exists); a later write touching the same skill self-heals it. Logged, not
  surfaced to the user, matching the "fails open" precedent elsewhere in the codebase
  (LLM spend attribution).
- **[Risk]** Additive-only merge means a skill removed from every atom that once justified
  it lingers in the profile. → **Mitigation:** accepted for this change; the candidate can
  still remove it manually today (same as any profile skill). Tracked as an open question,
  not silently ignored.
- **[Trade-off]** Read-modify-write against `Upsert`'s full-row-replace means two
  concurrent atom writes for the same user could race (last write wins on the non-skills
  fields the read stage captured). → **Mitigation:** the raced fields
  (`specializations`, `excluded_skills`, `location_preferences`) are not written by this
  path — `MergeSkills` reads them and writes them back unchanged — so a race only risks
  losing one of two concurrent *skill* additions, not corrupting other profile data.
  Bank atom writes for one user are not high-frequency or concurrent in practice (interactive
  chat, sequential CV import), so this is accepted rather than solved with a DB-level
  atomic array append.

## Migration Plan

- No data migration. Existing bank atoms are not backfilled into profiles — a candidate
  whose bank predates this change sees no gap they didn't already have (the removed button
  covered this before), and their profile catches up organically the next time any of their
  atoms is touched (edit, or a fresh CV-upload re-import). If a full backfill turns out to
  be wanted later, it is a one-off script over `AddAtom`'s merge logic, not part of this
  change.
- Deploy order: ship the Go-side sync first (additive, no behavior removed yet), confirm it
  fires correctly, then remove the tailor-page button in a follow-up deploy. This avoids a
  window where the button is gone but sync isn't live yet.
- Rollback: reverting the Go change is safe (profile just stops gaining the extra writer);
  reverting the button removal means re-adding `skillDiff.ts`, which is unaffected by the
  Go-side change and was not deleted from git history.

## Open Questions

- Should a skill ever be *removed* from the profile when no atom justifies it anymore? Left
  open — needs a decision on whether that is even desirable (a candidate may still want a
  skill in their search filters after editing away the one bullet that mentioned it).
