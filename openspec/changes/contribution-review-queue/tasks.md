## 1. Schema

- [x] 1.1 Add migration: `link_contributions` drop `NOT NULL` on `source`/`board`, replace status CHECK to add `review`, add partial unique index `ON (url) WHERE source IS NULL`.

## 2. Data access (sqlc)

- [x] 2.1 Add a query to record a review row (`url`, `submitted_by`, status `review`, source/board NULL) and a query to detect an existing review row by URL; regenerate sqlc.
- [x] 2.2 Extend the contribution repository with `RecordReview` and `ReviewExists` (or equivalent), mapping the nullable `source`/`board` columns to the domain `Contribution`.

## 3. Contribution service

- [x] 3.1 RED: test `Submit` — a valid unknown URL returns a `review` Contribution (status `review`, empty source/board, no error); non-URL garbage returns `ErrUnsupportedATS`; a URL already in the review queue returns `ErrBoardAlreadyContributed`.
- [x] 3.2 GREEN: change `Submit`'s unrecognized branch to record a review row (guarded by the valid-`http(s)`-URL check) instead of returning `ErrUnsupportedATS`; keep garbage → 422.

## 4. HTTP handler

- [x] 4.1 RED: handler test — an unknown valid link returns 201 with status `review` and awards no credit; a non-URL returns 422; a recognized novel board still 201 + credit.
- [x] 4.2 GREEN: gate `rewardContribution` on the recorded row being a recognized board (`status == "pending"`); return the review row as 201.

## 5. Frontend

- [x] 5.1 `types.ts`: make `Contribution.source`/`board` nullable and add `review` to the status union.
- [x] 5.2 `ContributeView.svelte`: on a `review` result show the "not a known ATS, we'll check by hand, not credited yet" confirmation; render review rows in the list with the URL host and an "under review · not credited" badge.

## 6. Docs

- [x] 6.1 Add a review-queue section to the `onboard-contributions` skill: how to drain `status='review'` rows, and the exact SQL to award the credit by contribution id + promote the row to `onboarded` once an adapter/board exists.

## 7. Verify

- [x] 7.1 `go build ./... && go vet ./... && go test ./...`; run the contribution + handler tests; confirm the contribute page renders the review state.
