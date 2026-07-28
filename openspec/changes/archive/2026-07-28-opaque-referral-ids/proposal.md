## Why

A referral offer and a referral request are each addressed by a sequential
number — `/me/referrals/offers/<id>`, `/me/referrals/incoming/<id>/cv`,
`/referrals/offers/<id>/proof`. This is the last user-owned resource in the
codebase still named by a counter; CVs and assistant sessions were made
unguessable earlier (`opaque-cv-ids`, `replace-assistant-runtime`).

The stakes here are higher than for a CV, and in an unusual way. A CV is read
only by its owner, so its authorization is "are you the owner". A referral
request is deliberately read by **someone else** — `GET /me/referrals/incoming/:id/cv`
streams the seeker's CV to an approved referrer of that company. That check is
not "is this yours" but "are you an approved referrer of the company this request
is addressed to", which is a genuinely more complicated question. The harder the
check, the more expensive it is to get wrong once — and a countable id turns one
mistake into "download every attached résumé" rather than one failed request.

The id also publishes volume: a seeker who files a request and sees `31` learns
how many referral requests the platform has ever received.

## What Changes

- **BREAKING** (internal surfaces only): `referral_offers.id` and
  `referral_requests.id` become random UUIDs. Every route that names one carries
  the UUID instead of a number, and the responses carry it as a string.
- A malformed id is reported as **not found** rather than as a bad request, so
  "not an id" and "not visible to you" stay one answer.
- **Unchanged:** who may see what. The moderator gate on offer review and the
  approved-referrer gate on an incoming request are exactly as before — this
  changes what a referral is called, not who may read it.

## Capabilities

### New Capabilities
<!-- none: this changes how an existing capability addresses its resources -->

### Modified Capabilities
- `employee-referrals`: offers and requests are addressed by unguessable ids, and
  a malformed id is reported as missing.

## Impact

**Backend:** a migration swapping both primary keys (nothing references either
table, so no dependent columns to carry); the referral queries and generated
code; `internal/referral`'s `Offer`/`Request` ids and repository signatures; the
referral handlers' `:id` parsing and response shapes.

**Frontend:** the referral id types in `web/src/lib/types.ts` and the calls in
`ReferralsView.svelte` / `api.ts`.

**Not affected:** the published CLI, the MCP server and the Claude Code plugin —
none of them touch referrals. Unlike `opaque-cv-ids`, this ships as one deploy of
one repository.
