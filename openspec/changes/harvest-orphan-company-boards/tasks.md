## 1. Corroboration gate in harvest-boards

- [ ] 1.1 Add a name-normalizing comparison to `cmd/harvest-boards` (case-fold, strip
      legal-form suffixes, collapse non-alphanumerics) with table tests covering agreement
      across case/punctuation/suffix differences and disagreement between distinct employers
- [ ] 1.2 Apply the gate in `probeAll`: reject a probed board whose reported name disagrees
      with the seed's expected employer, counting rejections separately from probe failures;
      leave behaviour unchanged when either name is absent
- [ ] 1.3 Report the mismatch count in the run's summary log alongside `found` and
      `probe-failures`

## 2. Orphan-company worklist

- [ ] 2.1 Add the sqlc query returning companies with open aggregator postings and no open
      non-aggregator posting, taking the requested aggregator set and the full aggregator set
      as separate parameters; run `make sqlc`
- [ ] 2.2 Cover the query with an integration test (build tag `integration`) asserting that
      an ATS-covered company is excluded, an aggregator-only company appears once, and
      narrowing the requested set does not admit a company held by another aggregator

## 3. Candidate derivation and seed emit

- [ ] 3.1 Implement candidate-slug derivation from company name and catalogue slug
      (hyphenated and unseparated renderings, legal-form suffixes stripped, per-company
      de-duplication, minimum length), unit-tested first
- [ ] 3.2 Implement seed emit: `[{board, company}]` pairs with the expected employer always
      present and no provider recorded, unit-tested against the shape `harvest-boards`
      parses
- [ ] 3.3 Wire `cmd/harvest-orphans` over `worker.Main`/`worker.Bootstrap`, taking the
      requested aggregators and the output path as flags and defaulting the aggregator set
      to the remote-jobs group

## 4. Verification

- [ ] 4.1 `go build ./... && go vet ./... && go test ./...` clean; `gofmt` clean
- [ ] 4.2 Run `harvest-orphans` against a local database and confirm the emitted seed parses
      through `harvest-boards`'s own seed loader
- [ ] 4.3 Dry-check the gate end to end on a known-bad pair (a live board belonging to a
      different employer) and confirm it is reported as a mismatch, not as a skip
