## 1. Schema

- [x] 1.1 Add migration `0046_referral_uuid_ids.sql`: swap `referral_offers.id` and `referral_requests.id` to random uuids in one transaction. Simpler than 0045 — nothing references either table, so there is no dependent column to carry and no foreign key to rebuild.
- [x] 1.2 Verify it against a real Postgres on a database seeded BEFORE the migration: both rows survive with uuid ids, the identity sequences are gone with the dropped columns, both primary keys are rebuilt, and all four CHECK constraints are still in place (the 0045 lesson — a CHECK naming a dropped column vanishes with it).

## 2. Query layer

- [x] 2.1 Add the `google/uuid` overrides for `referral_offers.id` and `referral_requests.id` and regenerate `internal/db`.
- [x] 2.2 Update `internal/referral`: `Offer.ID`, `Request.ID` and every repository and service signature that takes an id.

## 3. HTTP surface

- [x] 3.1 Parse `:id` as a uuid across the referral routes through a `referralPathID` helper; a malformed id is a 404, not a 400 — an unparseable opaque id is indistinguishable from one that does not exist.
- [x] 3.2 Render the ids as strings in the offer, request and incoming-request responses.
- [x] 3.3 Update the referral unit tests and the integration tests for the new id type.

## 4. Web

- [x] 4.1 Type a referral id as `string` in `web/src/lib/types.ts`; update `api.ts` (`withdrawReferralOffer`, `resolveReferral`, `decideReferralOffer`, `referralCvUrl`, `referralProofUrl`) to escape the id into the path, and the busy/withdrawing state in the two referral views.
- [x] 4.2 Run `svelte-check`, eslint, vitest and the production build.

## 5. Deploy

- [ ] 5.1 Apply the migration manually on production ahead of the API that reads it, under a `lock_timeout`.
- [ ] 5.2 Release the backend + web together. No published client addresses a referral, so nothing follows.
