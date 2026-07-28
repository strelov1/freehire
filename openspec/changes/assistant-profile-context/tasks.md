## 1. Contact-free projection of the structured résumé

- [x] 1.1 Add a contact-free view of `resumeextract.Structured` in
  `internal/resumeextract/structured.go` — a distinct type carrying only the
  professional fields (headline, location, summary, total years, experience,
  education, languages, skills, certifications, projects) and a constructor that
  projects a `Structured` onto it. Placing it in `structured.go` keeps it inside the
  file `cmd/gen-contracts` reads, so the TypeScript type is generated with it.
  Test: a `Structured` with every field populated projects to a value whose JSON has
  no `full_name`, `email`, `phone` or `links` key, and whose professional fields
  survive intact.
- [x] 1.2 Test that the projection is a whitelist, not a blacklist: the projection
  type's field set is enumerated explicitly, so a field added to `Structured` is
  absent from the projection until it is added there too.
- [x] 1.3 Move `matchanalysis.candidateContext` onto the same projection. It strips
  the same four fields today, but by deleting keys from an unmarshalled JSON map — a
  blacklist, so a field added to `Structured` reaches the LLM prompt verbatim. Route
  the JSON through the typed shape instead, so anything the projection does not name
  falls away.
  Test: a key outside the projection (personal data the blacklist could not have
  known about) does not reach the candidate context, and the projected fields survive.

## 2. The profile response carries the cv block

- [x] 2.1 Wire the résumé store into `profileHandlers` (`internal/handler/me_profile.go`,
  constructed in `internal/handler/handler.go`) so the profile read can reach the
  caller's structured résumé. Keep it optional — a handler built without it (as the
  narrow tests do) must still serve the profile.
  Test: `GetProfile` succeeds with a nil résumé store and returns `cv: null`.
- [x] 2.2 Add `cv` to `profileResponse` and populate it from the caller's current
  structured résumé, projected through 1.1.
  Test: a caller with a stored structured résumé gets the professional fields under
  `cv`, with no contact keys; a caller without one gets `cv: null`.
- [x] 2.3 Test that the résumé lookup failing degrades rather than breaking the read:
  the profile is still served with `cv: null`.

## 3. The profile read admits a key

- [x] 3.1 Move `GET /me/profile` from `mw.cookie` to `mw.cvKey`, leaving `PUT` and
  `DELETE` on `mw.cookie`. `mw.cvKey` rather than `mw.key` so the CV-tailoring
  agent — which holds a narrow `cv`-scoped key — can read the profile too; `mw.key`
  admits only full-scope keys and would answer it `403`.
  Test: a request authenticated by a full-scope API key alone reads the profile, and
  so does one authenticated by a `cv`-scoped key; either key gets `401` from `PUT`
  and from `DELETE`, and the stored profile is unchanged.
- [x] 3.2 Update the comment above the route block — it currently states the profile
  is cookie-only — to record the read/write split and why (a leaked key must not
  rewrite a profile).

## 4. Web contract

- [x] 4.1 Run `make gen-contracts` and add `cv` to the hand-written `UserProfile`
  interface in `web/src/lib/types.ts`, typed against the generated projection type.
  Verify: `go build ./...`, `go vet ./...`, `go test ./...`, and the web build.

## 5. Outside this repository

These are required before the assistant benefits and are tracked here for
visibility; they are edits to sibling repositories, not to this one.

- [ ] 5.1 `~/Projects/freehire-cli`: add a `freehire profile [--json]` command over
  `GET /me/profile`, with the client method and tests alongside the existing
  commands.
- [ ] 5.2 `~/Projects/freehire-agent`: in `crates/roy-management/src/http.rs`, pass
  the caller's `hire_token` into an assistant session's `extra_env` as
  `FREEHIRE_TOKEN`, beside the branch that already does this for tailoring sessions.
- [ ] 5.3 Update `using-freehire` in **both** copies —
  `~/Projects/freehire-cli/skills/using-freehire/SKILL.md` and
  `~/Projects/freehire-agent/docker/skills/using-freehire/SKILL.md` — to read the
  profile before interrogating the user, to send a user with no profile to
  `/my/profile` rather than collecting the same data in chat, and to treat a `401`
  as "start a new chat". Reconcile the drift between the two copies while editing.
