## Why

The experience bank can only be written from two places: the in-process assistant tools
(`experience_add`, only reachable from a chat turn) and the owner's own web UI edit form
(`PUT /me/experience/atoms/:id`, cookie-only — it corrects an existing entry, it cannot
create one). There is no way for a programmatic, API-key-authenticated caller — the
`freehire-cli`, or any future script the candidate runs themselves — to add a new
achievement or role outside a chat session. `GET /me/experience` already answers to a
full-scope key; only the write side is missing.

## What Changes

- Add `POST /me/experience/employments` and `POST /me/experience/atoms`, both admitted by
  the same full-scope key `GET /me/experience` already accepts (`mw.key`) — no new API-key
  scope. A leaked full-scope key already reaches everything else the CLI does (`cv edit`,
  `apply`); this adds one more additive-only capability to that same trust boundary rather
  than inventing a narrower one, which the key-minting flow does not support choosing today
  (every user key is minted `full`, unconditionally).
- A created atom is stamped `manual` provenance regardless of what the caller sends — the
  same rule `PUT /me/experience/atoms/:id` already applies to an edit. There is no chat
  transcript to check a `stated_in_chat` claim against outside a session, so `manual` (typed
  by the owner themselves, or on their behalf with their own key) is the only provenance an
  HTTP caller can honestly produce.
- `freehire-cli` gains `experience list`, `experience employments add`, and `experience atoms add`
  commands over the new and existing routes.

## Capabilities

### Modified Capabilities
- `experience-bank`: gains the ability to create an employment or an atom through the
  owner-facing HTTP surface, alongside the existing read/correct/remove.

## Impact

- Backend: `internal/handler/me_experience.go` (two new handlers, two new routes, the
  `experienceBankOwner` interface gains `AddAtom` and `CreateEmployment` — both already
  implemented on `*experience.Store`, no `internal/experience` changes needed).
- No migration, no new API-key scope.
- `freehire-cli` (separate repo): `internal/client/experience.go` (new),
  `internal/cli/experience.go` (new) — `experience list`, `experience employments add`,
  `experience atoms add`.
