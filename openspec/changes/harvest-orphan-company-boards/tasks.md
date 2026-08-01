## 1. Corroboration gate in harvest-boards

- [x] 1.1 Add a name-normalizing comparison to `cmd/harvest-boards` (case-fold, strip
      legal-form suffixes, collapse non-alphanumerics) with table tests covering agreement
      across case/punctuation/suffix differences and disagreement between distinct employers
- [x] 1.2 Apply the gate in `probeAll`: reject a probed board whose reported name disagrees
      with the seed's expected employer, counting rejections separately from probe failures;
      leave behaviour unchanged when either name is absent
- [x] 1.3 Report the mismatch count in the run's summary log alongside `found` and
      `probe-failures`

## 1a. Review fallout — the prober name contract

Review of task 1 found the gate resting on an inference that two probers break: "the
platform reported no name" was read as "the returned name equals the board id", but
`workdayProber` returns the tenant and `opencatsCompanyName` returns the host, neither of
which equals the board id. Since `cmd/harvest-ats` and `cmd/harvest-role` already emit seeds
carrying an expected employer, a Workday run would have rejected every live board.

- [x] 1a.1 Replace the inference with an explicit contract: a prober returns `""` when the
      platform reports no employer name of its own. Convert every prober that returns the
      slug as a fallback, and fix `workdayProber` and `opencatsCompanyName` to stop returning
      a derived token; drop `reportedName` and the `orSlug` fallback
- [x] 1a.2 Regression-test the contract: a prober reporting no name keeps the seed's label,
      and a prober returning a slug-derived token (the Workday tenant shape) is not gated
- [x] 1a.3 Extract the employer name where the platform does publish one but the prober
      discards it — recruitee (`company_name`) and breezy (`company.name`), both verified
      live. Lever, Ashby, Personio and BambooHR publish none and stay ungatable
- [x] 1a.4 Fail the run when every candidate was rejected as a name mismatch, mirroring the
      existing all-probes-failed guard, so a systematically broken gate cannot exit 0 silently
- [x] 1a.5 Loop the legal-suffix strip instead of stopping at the first match, add the
      missing forms (`plc/ag/sa/nv/pty/kg/llp/srl/a-s`), and fold diacritics
- [x] 1a.6 Treat an expected name that normalizes to empty as no expectation rather than as
      a mismatch
- [x] 1a.7 Correct `design.md`: existing seeds from `harvest-ats`/`harvest-role` DO carry an
      expected employer, and the set of ungatable providers is far wider than jazzhr/jobvite

## 2. Orphan-company worklist

- [x] 2.1 Add the sqlc query returning companies with open aggregator postings and no open
      non-aggregator posting, taking the requested aggregator set and the full aggregator set
      as separate parameters; run `make sqlc`
- [x] 2.2 Cover the query with an integration test (build tag `integration`) asserting that
      an ATS-covered company is excluded, an aggregator-only company appears once, and
      narrowing the requested set does not admit a company held by another aggregator

## 3. Candidate derivation and seed emit

- [x] 3.1 Implement candidate-slug derivation from company name and catalogue slug
      (hyphenated and unseparated renderings, legal-form suffixes stripped, per-company
      de-duplication, minimum length), unit-tested first. The name folding moved to
      `internal/normalize` (`CompanySlug`/`CompanyKey`/`SameCompany`) once the harvest gate
      and the candidate generator both needed it — duplicating the legal-form list across two
      binaries was not an option
- [x] 3.2 Implement seed emit: `[{board, company}]` pairs with the expected employer always
      present and no provider recorded, unit-tested against the shape `harvest-boards`
      parses
- [x] 3.3 Wire `cmd/harvest-orphans` over `worker.Main`/`worker.Bootstrap`, taking the
      requested aggregators and the output path as flags and defaulting the aggregator set
      to the remote-jobs group

## 4. Verification

- [x] 4.1 `go build ./... && go vet ./... && go test ./...` clean; `gofmt` clean
- [x] 4.2 Run `harvest-orphans` against a local database and confirm the emitted seed parses
      through `harvest-boards`'s own seed loader
- [x] 4.3 Dry-check the gate end to end on a known-bad pair (a live board belonging to a
      different employer) and confirm it is reported as a mismatch, not as a skip

## 4a. Second review fallout

Review of sections 2–4 found two more silent, one-directional defects.

- [x] 4a.1 `sources.AllAggregatorProviders` answers aggregator membership independently of
      credentials; the crawl registry keeps answering "which can this process crawl". Both
      `cmd/harvest-orphans` and `cmd/reindex`'s suppression pass use it
- [x] 4a.2 Contest candidates on the folded name, so one employer's two catalogue spellings
      keep the board id they agree on instead of discarding it
- [x] 4a.3 Bound the worklist scan with a `statement_timeout` on a pinned connection
- [x] 4a.4 Cover the branches no test could fail on: the folds-to-nothing gate, the modal
      display name, and a transliteration test whose fixture made it vacuous
