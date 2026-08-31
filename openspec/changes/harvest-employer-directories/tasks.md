## 1. Justify the rest of the work

- [ ] 1.1 Take 60 Ethiojobs company pages spread across the directory, read their
  `website` fields by hand or with a throwaway script, and run `harvest-ats resolve`
  over the ones that have one. Record how many resolve to a detectable ATS board.
- [ ] 1.2 Write the measured detected-board rate into the proposal beside the 33%
  website rate. **If it is negligible, stop here and close the change** — the design's
  first risk says this is the gate, not a formality.

## 2. The directory worklist

- [ ] 2.1 Write the failing test for `parseCompany` against a saved Ethiojobs company
  page fixture: a page with a website yields name and website.
- [ ] 2.2 Write the failing tests for the refusal cases — empty `website`, absent
  `website` key, absent `__NEXT_DATA__`, truncated payload — each yielding `ok=false`
  without a panic. Then implement `parseCompany` in
  `cmd/harvest-ats/ethiojobs.go`.
- [ ] 2.3 Write the failing test for the sitemap parser against a saved fixture,
  including that image and asset `<loc>` entries sharing the file are ignored. Then
  implement `companyURLs`.
- [ ] 2.4 Define the `employerDirectory` interface in
  `cmd/harvest-ats/directory.go` and make the Ethiojobs implementation satisfy it.
- [ ] 2.5 Write the failing test that a run whose pages all parse to `ok=false` exits
  non-zero, then implement the guard — an empty worklist must not read as "no new
  employers".
- [ ] 2.6 Implement the paged run: fetch the company URLs, fetch each page under the
  existing worker and timeout discipline, parse, then `filterUnmatched` +
  `dedupeByWebsite`. Log progress — the sitemap alone took 68s when measured.
- [ ] 2.7 Add the `directory` subcommand to `cmd/harvest-ats/main.go` beside
  `extract`, `resolve` and `universities`, and extend the command's doc comment.
- [ ] 2.8 Write the failing test that the directory path emits the same JSON shape
  `resolve` reads, then confirm it passes — the pin that keeps the three worklists
  interchangeable.

## 3. The first run

- [ ] 3.1 Run `harvest-ats directory` against the current company-slug set, then
  `harvest-ats resolve` over its output.
- [ ] 3.2 Run `cmd/harvest-boards` per provider over the produced seed files, paced.
  Review the diff to `sources/*.yml` before committing — the tool proposes, a human
  commits.
- [ ] 3.3 Record in the commit message how many candidates were probed, kept, rejected
  for a name mismatch, and unreachable, so the run's yield is on the record beside the
  boards it added.

## 4. Verify

- [ ] 4.1 `gofmt -l .` prints nothing; `go vet ./...`; `go test ./...`;
  `go vet -tags=integration ./...`.
- [ ] 4.2 `golangci-lint run` clean for the new files.
- [ ] 4.3 Confirm no test reaches the network: the parsers run against fixtures only.
- [ ] 4.4 After the boards land and one ingest cycle runs, re-read the Ethiopia,
  Kenya, Uganda and Nigeria catalogue counts and record them against the baseline in
  `docs/seo-baseline.md` (158 / 713 / 119 / 2,937).
