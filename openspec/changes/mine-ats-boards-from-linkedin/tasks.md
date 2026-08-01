## 1. Board confirmation by expected posting id (`cmd/harvest-boards`)

- [x] 1.1 Extend `seedItem` in `seed.go` with an optional `expect_id`, keeping both existing
  seed shapes (array of strings, array of objects) parsing exactly as before
- [x] 1.2 Add an optional `idProber` interface beside `prober` in `prober.go` that reports
  the ids of a board's live postings, leaving the `prober` interface itself untouched
- [x] 1.3 Implement `idProber` on the probers whose single request already yields the board's
  complete set of live posting ids: greenhouse, lever, ashby, recruitee. SmartRecruiters
  (probed with `limit=1`) and Teamtailor (first page only) see a partial list, where a missing
  id would not mean an absent posting — they stay inert
- [x] 1.4 Wire confirmation into the probe loop: an expected id decides the candidate,
  outranks the employer-name comparison, and is inert when the provider reports no ids
- [x] 1.5 Count and report id mismatches separately from unreachable candidates, as name
  mismatches already are

## 2. Wider board detection during resolution (`cmd/harvest-ats`)

- [ ] 2.1 Replace the three `atsdetect.Detect` calls in `resolve.go` with detection through
  `internal/boardresolve`, keeping the careers-page walk (paths, careers link, one deeper
  hop) and the best-effort skip-on-error behaviour unchanged
- [ ] 2.2 Cover the newly reachable cases: a careers page linking an ATS outside the narrow
  scan, and a career site whose own host is the board

## 3. Offline candidate slugs for unresolved companies (`cmd/harvest-ats`)

- [ ] 3.1 Add a pure function deriving candidate board slugs from a company's domain,
  profile slug and name, bounded to a small fixed number and performing no I/O
- [ ] 3.2 Add a pure function narrowing an ATS-native posting id to the providers its shape
  is consistent with, yielding nothing for shapes that narrow to none
- [ ] 3.3 Emit derived candidates into each narrowed provider's seed carrying the expected
  posting id, only when the careers walk found no board and an id is present
- [ ] 3.4 Carry the new input fields (`linkedin`, `external_id`) through the resolve input
  without disturbing the existing `{name, website}` worklists

## 4. LinkedIn discovery tool (`cmd/harvest-linkedin`)

- [ ] 4.1 Parse the query worklist YAML (keywords, location, jobage, pages) with bounded
  defaults, refusing an entry that names no market
- [ ] 4.2 Parse the public search listing into postings carrying employer name, employer
  profile URL and posting URL, one card at a time so a malformed card costs one posting
- [ ] 4.3 Parse a posting's JSON-LD for the ATS-native identifier, and an employer profile's
  JSON-LD for the website
- [ ] 4.4 Collapse postings to one candidate per employer and drop candidates whose
  normalized-name slug is in the supplied company-slug set, both before any detail fetch
- [ ] 4.5 Emit surviving candidates as `{name, website, linkedin, external_id}` JSON on
  stdout, omitting those with no website and skipping-with-log those that fail to fetch
- [ ] 4.6 Warn on a query that returns no postings and exit non-zero when every query does
- [ ] 4.7 Wire the run through `sources.NewClient()` and the shared pacer, with a `-pace`
  flag and a conservative default, and bootstrap the command per `worker.Bootstrap`
  convention

## 5. Worklist file and documentation

- [ ] 5.1 Add `harvest/linkedin-queries.yml` with a small starter set of keyword×market
  queries
- [ ] 5.2 Document the three-step run (`harvest-linkedin` → `harvest-ats resolve` →
  `harvest-boards`) in the command's package comment, matching how the other `harvest-*`
  tools document themselves
- [ ] 5.3 Record the expected-id seed field where the harvest worklist conventions are
  described, so a future seed source knows it can supply one

## 6. Verification

- [ ] 6.1 `go build ./... && go vet ./... && go test ./...` clean
- [ ] 6.2 Live smoke run of `harvest-linkedin` on two or three queries; confirm candidates
  are produced and report how many survive the catalogue filter
- [ ] 6.3 Carry that output through `harvest-ats resolve` and `harvest-boards` for one
  provider, and review the resulting `sources/*.yml` diff before proposing it
