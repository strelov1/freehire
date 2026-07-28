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

- [x] 3.1 Move `GET /me/profile` from `mw.cookie` to `mw.key`, leaving `PUT` and
  `DELETE` on `mw.cookie`, so the `freehire` CLI can read a profile with the user's
  own API key while a leaked key can never rewrite one. (The in-app agent does not
  come through here — its tools run in-process with the user id in hand.)
  Test: the gate behind each profile route, pinned against the real `register()`.
- [x] 3.2 Update the comment above the route block — it currently states the profile
  is cookie-only — to record the read/write split and why (a leaked key must not
  rewrite a profile).

## 4. Web contract

- [x] 4.1 Run `make gen-contracts` and add `cv` to the hand-written `UserProfile`
  interface in `web/src/lib/types.ts`, typed against the generated projection type.
  Verify: `go build ./...`, `go vet ./...`, `go test ./...`, and the web build.

## 5. The get_profile tool

- [x] 5.1 Add `get_profile` to the agent's discovery tools
  (`internal/handler/assistant_profile_tool.go`), built on `profileHandlers` so the
  tool and `GET /me/profile` share one assembly and cannot drift. `assistantHandlers`
  gains the profile handlers; `newAssistantHandlers` and its call site follow.
  Test: the tool is registered for every session; its result carries the profile and
  the CV's professional fields and none of the four contact fields.
- [x] 5.2 A caller with no saved profile gets a result that names the profile page,
  not an error and not an empty profile the model might read as "no preferences".
  Test: `Run` returns successfully and the payload mentions `/my/profile`.
- [x] 5.3 Teach the chat prompt to call it before interrogating the user, to say what
  it searched on, and to send a profile-less user to `/my/profile`.
  Test: the chat prompt mentions `get_profile`.

## 6. Documentation

- [x] 6.1 Update `internal/resumeextract/AGENTS.md` for the projection and its two
  serving surfaces, and `internal/handler/AGENTS.md` for the new tool.

Superseded: an earlier revision of this change routed the assistant through the
`freehire` CLI with a credential injected into a sandbox. That runtime was replaced
by the in-process agent (#1165) before this change was written; the tool above is the
same feature against the runtime that actually exists.
