## 1. Measure first, and keep the measurement

- [x] 1.1 Seed a 500-application board with 5 KB descriptions and record where the cost is:
      payload size, the description's share of it, the scan with and without the column, and
      the endpoint's own time (`TestMeasureBoardLoad`).
- [x] 1.2 Turn it into a regression guard: the listing carries no description, and the payload
      for 500 rows stays under a stated ceiling. Verify by mutation — put the description back
      and watch it fail.

## 2. The listing reads only what a card draws

- [x] 2.1 Add the card listing query to `internal/db/queries/user_jobs.sql` reading only the
      card's columns from `jobs`, and run `make sqlc`.
- [x] 2.2 Collapse the three correlated `emails` subqueries into one `LATERAL` returning the
      count, the newest `received_at` and the pending-suggestion boolean in a single pass.
- [x] 2.3 Add the card wire type and serialize it from `internal/handler/me_tracking.go`,
      keeping every interaction field the row already carries.
- [x] 2.4 Confirm the silence derivation is unchanged: `last_activity_at` still reads
      `GREATEST(applied_at, newest linked mail)` and still excludes the follow-up and the CV open.

## 3. Indexes — measured and dropped from scope

- [x] 3.1 Seed a realistic mailbox (5 000 messages, 167 linked) and measure the listing with and
      without the candidate indexes. Result: 2.62 ms vs 2.79 ms — noise. The two orders of
      magnitude came from `ANALYZE` after the bulk insert, a fixture artefact.
- [x] 3.2 Record the numbers in the proposal and drop the migration: an index that costs every
      write to buy nothing measurable on the read is infrastructure before need.

## 4. The drawer reads the posting from the detail response

- [x] 4.1 Take the description and the full facet row from `getTrackedApplication`, which the
      drawer already fetches, instead of from the listing row.
- [x] 4.2 Give the board card its own tag function over the flat card facets, leaving
      `cardTags` to the catalogue surfaces that still hold a full `Job`.
- [x] 4.3 Update `web/src/lib/types.ts` and everything typed against the listing's `job`.

## 5. Documentation

- [x] 5.1 Update `docs/API.md` and `web/src/lib/docs/api-spec.ts` for the card shape, stating
      where the full posting lives.
- [x] 5.2 Note the split in `internal/userjob/AGENTS.md`: the listing serves cards, the detail
      read serves the posting.
- [ ] 5.3 Run both Go suites, all four web gates, and the design-system token ratchet before
      pushing.
