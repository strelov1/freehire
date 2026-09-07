## 1. The denial dictionary

- [x] 1.1 In `internal/dict/location/workmode_test.go`, write the failing table for
      `RemoteContradicted`: every denial phrase fires; the two ADP `"fully on-site for the
      first 90 days"` sentences (verbatim) do NOT; the NVIDIA `"100% on-site based at
      either our Dallas or Houston"` sentence does; and "remote team", "hybrid cloud",
      "parking on-site only", "must be onsite for quarterly planning" do NOT.
- [x] 1.2 Add `remoteDenialPhrases` and `RemoteContradicted` to
      `internal/dict/location/workmode.go`, with the doc saying why the list is separate
      from `descriptionWorkModePhrases` and why `fully on-site` / `on-site only` are absent.

## 2. The contradiction tier

- [x] 2.1 In `internal/job/jobderive/jobderive_test.go`, write the failing cases: a
      structured `remote` overridden to `onsite`; a location-derived `remote` (`"US, TX,
      Remote"`) overridden; a `hybrid` result left alone; a `remote` with no denial left
      alone.
- [x] 2.2 Apply the override in `jobderive.Derive` after the three-tier resolution, with the
      comment saying why it outranks the structured signal and why it only ever reads
      `remote`.

## 3. `hydrated_at`

- [x] 3.1 Add the migration: `ALTER TABLE jobs ADD COLUMN hydrated_at timestamptz`, with the
      comment saying it means "last written from a freshly fetched body" and that NULL reads
      as stale. Run `pnpm check:sql`.
- [x] 3.2 Set `hydrated_at = now()` in `UpsertJob` and `RefreshUnchangedJob` in
      `internal/platform/db/queries/jobs.sql` — and NOT in `TouchJob`, with the comment
      saying why. Run `make sqlc`.

## 4. The staleness arm

- [x] 4.1 Widen `ExistingExternalIDs` and `ExistingExternalIDsByBoard` with the staleness
      slot arm, documenting `hashtext` determinism and the day-of-year slot. Run `make sqlc`.
- [x] 4.2 Add `bodyRefreshFor` to `cmd/ingest/main.go` reading `BODY_REFRESH_DAYS` /
      `BODY_REFRESH_SLICE`, unset = disabled, resolved the same way its neighbour
      `hydrationRetryWindowFor` resolves its own knob, with unit tests for unset, valid,
      unparseable, and a slice set without days.
- [x] 4.3 Thread it through `newDBStore` into the two queries; when disabled, pass the
      parameters that make the arm a no-op so there is one predicate, not two code paths.
      Group it with the two existing seen-set knobs into a `seenPolicy` rather than growing
      the constructor to seven positional arguments.
- [x] 4.4 Extend the `internal/platform/db` integration test with a stale-`hydrated_at` row
      (withheld) and a fresh one (kept), both in the run's slot.

## 5. Ship

- [x] 5.1 `gofmt -w`, `go vet ./...`, `go test ./...`, `go vet -tags=integration ./...`.
- [x] 5.2 Add the two knobs to `AGENTS.md`'s `cmd/ingest` worker note.
- [x] 5.3 Open the PR referencing freehire#2555.
