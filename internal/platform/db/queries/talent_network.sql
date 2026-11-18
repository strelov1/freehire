-- name: GetTalentNetworkVisibility :one
-- The caller's own Talent Network opt-in state, for the owner-facing settings toggle.
-- talent_handle rides along so the page can render the public URL without a second
-- round-trip. It is NULL until the first join — a non-member has no card to link to —
-- unlike the visibility, which every row carries because 'off' is the column default.
--
-- `listed` answers the question the settings page actually has to ask: not "am I a
-- member" but "does a visitor see me". They come apart, and the gap is a live trap — a
-- candidate who joins before uploading a CV is a member with a handle whose card 404s,
-- so a page reading membership alone tells them their profile is up when it is not.
--
-- It repeats ListTalentNetworkMembers' predicate, which is a duplication worth naming:
-- the two must be changed together. It is not shared because the catalogue's version
-- selects rows and this one describes one row, and a caller cannot ask the first
-- "and what about me".
--
-- COALESCE is the difference from that WHERE. There, a NULL comparison simply drops the
-- row. Here the same comparison is a column scanned into a Go bool, and
-- `resume_structured_uploaded_at = resume_uploaded_at` is NULL when the extract stamp is
-- missing — which is the ordinary state of a CV that was uploaded and not yet parsed.
-- TRUE AND NULL is NULL, so a member in that state used to 500 the settings read. Not
-- listed is the answer, the same one the catalogue already gives by excluding them.
SELECT talent_network_visibility,
       talent_handle,
       COALESCE((talent_network_visibility <> 'off'
        AND talent_handle IS NOT NULL
        AND resume_uploaded_at IS NOT NULL
        AND resume_structured_uploaded_at = resume_uploaded_at), false)::boolean AS listed
FROM users
WHERE id = $1;

-- name: SetTalentHandleIfUnset :execrows
-- Claims a freshly minted catalogue handle for a candidate who does not have one yet.
--
-- The `talent_handle IS NULL` predicate is the whole mechanism, and it does two jobs.
-- It makes the mint idempotent — a member who leaves and rejoins keeps the handle they
-- already shared, and a second concurrent join claims nothing — and it makes the
-- statement's own result the answer: 0 rows means somebody already has one, which the
-- caller reads rather than re-querying and racing again.
--
-- A collision with ANOTHER account's handle surfaces as a unique-violation from
-- users_talent_handle_key (migration 0150), not as 0 rows. The caller mints a new suffix
-- and retries — the same shape internal/identity/accounts uses to allocate a username.
UPDATE users
SET talent_handle = $2
WHERE id = $1
  AND talent_handle IS NULL;

-- name: SetTalentNetworkVisibility :exec
-- Owner-scoped write of the caller's Talent Network membership ('off' or 'anonymous'
-- since migration 0148). Does not touch talent_handle: the public URL stays stable
-- across a round trip through 'off', so a candidate who already shared it once — or who
-- leaves and rejoins — never has to reshare a new one.
--
-- The value is NOT validated here. Its authority is the CHECK constraint on the column;
-- the handler mirrors that set for a cheap 400, and this statement is the third place
-- the vocabulary would have to be repeated for no gain.
UPDATE users
SET talent_network_visibility = $2
WHERE id = $1;

-- name: ListTalentNetworkMembers :many
-- Every member the public catalogue may show, in one read. The caller projects each row
-- through talentnetwork.ProjectCard and holds the result as a snapshot — see that
-- package's doc for why the whole set is read at once rather than filtered in SQL: the
-- category and seniority a card is filtered by do not exist as columns, they are derived
-- from the job title by a dictionary that changes weekly.
--
-- The predicate is the membership rule and nothing else:
--
--   * not 'off' — the candidate asked to be found;
--   * a minted handle — without one there is no URL to link the card to, so a row in
--     this state is mid-join, not a member;
--   * the stamp gate (resume_structured_uploaded_at = resume_uploaded_at, both set) —
--     the same "this structure still describes the CV on file" rule the rest of the
--     product applies. Without it most cards would be somebody's previous CV.
--
-- LEFT JOIN, because a candidate can join before ever saving a profile: a missing
-- user_profiles row is empty facets, not a missing member.
--
-- Ordered here rather than by the caller so the snapshot arrives sorted, and TOTALLY:
-- two members sharing a timestamp would otherwise order arbitrarily, and an arbitrary
-- order across pages silently drops some people and repeats others.
SELECT u.talent_handle,
       u.timezone,
       COALESCE(u.resume_cities, '{}')::text[] AS cities,
       u.resume_structured,
       u.candidate_contacts,
       u.resume_structured_uploaded_at,
       COALESCE(p.specializations, '{}')::text[] AS specializations,
       -- Whether the member has an uploaded headshot, never the object key itself: the
       -- catalogue card links to the blurred photo route by the member's own handle
       -- (talentnetwork.CatalogueMember.HasPhoto), so a card can skip requesting a photo
       -- that GetPhoto would only 404 for.
       (u.photo_object_key IS NOT NULL AND u.photo_object_key <> '')::boolean AS has_photo
FROM users u
LEFT JOIN user_profiles p ON p.user_id = u.id
WHERE u.talent_network_visibility <> 'off'
  AND u.talent_handle IS NOT NULL
  AND u.resume_uploaded_at IS NOT NULL
  AND u.resume_structured_uploaded_at = u.resume_uploaded_at
ORDER BY u.resume_structured_uploaded_at DESC, u.talent_handle DESC;

-- name: GetTalentNetworkMemberByHandle :one
-- One member's card, by the handle in the public URL. Same predicate as the list, so a
-- handle nobody holds, a member who has left, and one whose extract has gone stale all
-- come back as pgx.ErrNoRows — which the handler renders as the one 404. Deciding it
-- here rather than in the caller is deliberate: three ways to be absent and one way to
-- say so is a rule that cannot be half-applied.
--
-- Read against the DATABASE, never the snapshot the list is served from. A candidate who
-- leaves must stop resolving immediately, not when the snapshot next refreshes.
SELECT u.talent_handle,
       u.timezone,
       COALESCE(u.resume_cities, '{}')::text[] AS cities,
       u.resume_structured,
       u.candidate_contacts,
       u.resume_structured_uploaded_at,
       COALESCE(p.specializations, '{}')::text[] AS specializations,
       (u.photo_object_key IS NOT NULL AND u.photo_object_key <> '')::boolean AS has_photo
FROM users u
LEFT JOIN user_profiles p ON p.user_id = u.id
-- The ::text cast is load-bearing, not decoration: talent_handle is nullable, so without
-- it sqlc types the argument as pgtype.Text and every caller has to wrap a plain string
-- it already knows is present.
WHERE u.talent_handle = sqlc.arg(handle)::text
  AND u.talent_network_visibility <> 'off'
  AND u.resume_uploaded_at IS NOT NULL
  AND u.resume_structured_uploaded_at = u.resume_uploaded_at;

-- name: GetTalentNetworkMemberUserIDByHandle :one
-- The same "is this handle a current member" predicate as
-- GetTalentNetworkMemberByHandle, but naming only the user id — for the public photo
-- route, which needs a headshot owner, never a public card. Kept as its own query
-- rather than widening ByHandle's result: the id is not part of the public projection
-- and has no reason to travel through the same path that assembles one.
SELECT u.id
FROM users u
WHERE u.talent_handle = sqlc.arg(handle)::text
  AND u.talent_network_visibility <> 'off'
  AND u.resume_uploaded_at IS NOT NULL
  AND u.resume_structured_uploaded_at = u.resume_uploaded_at;
