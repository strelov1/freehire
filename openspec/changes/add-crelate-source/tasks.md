## 1. Adapter

- [x] 1.1 Add `crelate.go`: provider key `crelate`, `<portalSlug>:<organizationId>` board parsing (both parts required, else a clear board-level error), the `GetAllJobs` request with the URL-encoded `requestEnvelope`, and the `{Jobs, IsError, ErrorMessage}` response struct (treat `IsError:true` as a board failure). Cover board validation and the IsError path with a fake HTTP client.
- [x] 1.2 Map a posting onto `Job`: `Id`→`ExternalID` (drop empty-id postings), `Title`, human URL `jobs.crelate.com/portal/<slug>/job/<JobCode>`, `City`/`State`/`Country`→`Location`, `LastPostedOnDate`→`PostedAt`, sanitized `Description`. Cover the mapping and the empty-id drop with tests.
- [x] 1.3 Mark the adapter `aggregator()` and resolve each `Job.Company` from the posting's `CompanyName`, falling back to the configured company. Cover both paths with tests.

## 2. Wiring

- [x] 2.1 Register `NewCrelate` in `sources.All`.
- [x] 2.2 Add `sources/crelate.yml` seeded with the Career Tree Network board (`careertree:ec546cba-84d5-4d8a-97e5-52e8ef47db08`) and a header documenting the board format and where the OrganizationId GUID lives; confirm the file validates against the registry.

## 3. Verify & ship

- [x] 3.1 Validate end-to-end against the live portal through the real adapter: fetch the seeded board and confirm postings, location coverage, and posted dates.
- [ ] 3.2 Ship: open a PR, merge to main after CI is green, deploy on host-2 (`release.sh`), install and enable the `freehire-ingest@crelate` hourly timer, run one ingest, and confirm the jobs land in the prod database.
