## Context

`internal/handler/me_experience.go` already serves the owner's whole bank
(`GET /me/experience`, full-scope key or cookie) and lets them correct or remove any entry
(`PUT`/`DELETE`, cookie-only — the file's own comment explains why: "a key that leaks out
of a script's environment must not edit or erase someone's career"). `Store.AddAtom` and
`Store.CreateEmployment` (`internal/experience/store.go`) already exist and are exercised
today only by the in-process assistant tools (`experience_add`, `internal/handler/assistant_experience_tools.go`),
which run inside a chat turn and can check a claim's `said` text against the session
transcript to decide `stated_in_chat` vs `agent_inferred` provenance.

`freehire-cli` is a separate repo (`github.com/strelov1/freehire-cli`), a thin HTTP client
authenticated by one full-scope API key (`fhk_...`) that already drives `cv edit`, `apply`,
`save`, etc. It has no bank-write command today because no route exists for it to call.

## Goals / Non-Goals

**Goals:**
- Let a full-scope key create a new employment or a new atom, the same way it can already
  read every existing one.
- Keep the provenance rule the honest wall depends on intact: an atom created this way must
  never be indistinguishable from one the candidate stated in a chat the server can verify.

**Non-Goals:**
- A new, narrower API-key scope for bank writes. Investigated and rejected for this change:
  scope selection at key-creation time does not exist for user-facing keys at all today
  (`mintAPIKey` hardcodes `ScopeFull`) — building it would mean a migration, a new
  `createAPIKeyRequest` field, and new UI, for a narrower boundary around a capability
  (adding) that is strictly less destructive than the edit/delete this same key can already
  reach through the web UI's own key. If a future change wants a genuinely narrower key
  (e.g. so a compromised automation script can add but never delete), that is its own
  proposal, not a prerequisite for this one.
- Letting the write side accept `stated_in_chat` or `cv_import` provenance from the caller.
  Only the server may assert those — see Decisions.

## Decisions

- **Both new routes sit behind the SAME `mw.key` the existing `GET /me/experience` already
  uses** (full scope, no new middleware instance). A full-scope key already carries the
  power to `cv edit` a fabricated bullet's summary line or `apply` on the candidate's
  behalf; gating an *additive* bank write behind anything narrower would protect a boundary
  the same key already crosses everywhere else in the CLI.
- **The handler forces `Provenance: experience.ProvenanceManual`; any value the caller sends
  is discarded** — identical to what `UpdateAtom` already does. This is the one place the
  honest wall is enforced on this new write path: `Provenance.Publishable()` treats `manual`
  as citable into a CV precisely because it means "the owner typed this themselves, outside
  a verified chat", which is exactly what an authenticated HTTP POST is. Accepting a
  caller-supplied `stated_in_chat` here would let a full-scope key mint an atom that *looks*
  chat-verified without ever having been said in a transcript the server checked.
- **`CreateEmployment`/`AddAtom` reuse the Store's existing `Sanitize`/`Validate` — no new
  validation is written.** Both already run on the assistant's write path; a second,
  hand-rolled check here would be the exact kind of forked validation the codebase's own
  conventions warn against (parallel to `internal/cv`'s token-coverage script comment about
  a forked detector drifting from the one it copied).
- **Response is `201` with `{"data": <created entity>}`**, matching the wire convention
  every other creating endpoint in this handler layer uses (`POST /me/cvs`, `jobs add`,
  etc.) — no new envelope shape.

## Risks / Trade-offs

- [A leaked full-scope key can now also spam the bank with junk `manual` atoms, not just
  read it] → Mitigation: the same key already lets the leak holder do worse (edit a CV
  bullet's text directly, mark applications applied). This does not open a new class of
  harm, it adds one more thing an already-critical leak can do. `Sanitize`/`Validate` bound
  size and shape, and the owner can still delete a junk atom from the web UI
  (`DELETE /me/experience/atoms/:id`, cookie-only, unaffected by this change).
- [A future reader of `me_experience.go` assumes the write side is symmetric with the read
  side's auth (both full-scope-or-cookie) and widens `PUT`/`DELETE` to `mw.key` by analogy,
  re-opening the exact hazard the file's own comment warns about] → Mitigation: none beyond
  the existing comment; noted here so this change's diff doesn't read as license to do that
  in a later one.
