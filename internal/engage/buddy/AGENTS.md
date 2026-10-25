# Interview buddies

## Scope
The opt-in peer network (openspec/changes/interview-buddies): membership under a generated
handle, similar members, a feed of other members' search progress, and one-to-one chat with
an invitation step. Layer 7 (`engage`); tables are `buddy_*` (migration 0197), queries in
`internal/platform/db/queries/buddies.sql`. HTTP lives in `internal/api/handler`, not here.

## Always true
- **Anonymity.** A member is a handle. Nothing they typed into their CV reaches another
  member, except role titles through `talentnetwork.ProjectCard`. The buddy handle is
  minted separately from the Talent Network and community handles so that no two of them
  can be joined. Notifications carry no message text and no account detail.
- **Feed vocabulary.** The feed shows `applied`, and `stage_set` with signal `interview` or
  `offer`, at day precision, and nothing else. `accepted`, `rejected`, `withdrawn` and
  `expired` never appear: "took the offer" is the most identifying fact in the ledger. An
  event is dropped when it is retracted, hidden by its owner (`buddy_hidden_applications`), or the
  owner has left or is blocked in either direction.
- **One conversation per pair**, stored as `user_low < user_high`. Leaving closes every
  pending or open conversation the member is in; the row and handle survive, so rejoining
  restores the handle.
- **Account deletion** is the foreign keys: every reference to `users` cascades, and a
  new one must be indexed on the referencing side (`TestEveryUserForeignKeyIsIndexed`).
- A conversation the caller is not part of is "not found", never "forbidden".
