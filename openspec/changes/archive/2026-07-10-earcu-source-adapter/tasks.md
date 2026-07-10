## 1. Adapter core (RSS crawl)

- [x] 1.1 Add `internal/sources/earcu.go`: `earcuHTTP interface { XMLGetter; HTMLGetter }`, `earcu` struct, `NewEarcu`, and `Provider() string { return "earcu" }` (mirror `personio.go`).
- [x] 1.2 Define the RSS decode structs (`<rss><channel><item>` with title/link/guid/pubDate/description) and `Fetch` building `https://<board>/jobs/rss` via `GetXML`, returning one `Job` per item.
- [x] 1.3 Parse `ExternalID` from the `/jobs/vacancy/<slug>/<id>/description/` link path; drop items with no extractable id.
- [x] 1.4 Parse posted-at from `<pubDate>` (RFC1123 with `Z`-zone tolerance; nil on failure) and clean the title by stripping a trailing ` (\d+)` reference suffix.

## 2. Description + location

- [x] 2.1 Extract the inline body from the `rssjobdesc` block in the (unescaped) `<description>` and `sanitizeHTML` it; parse `Location:`/`Country:` out of the `Key: value` prefix for `Job.Location`.
- [x] 2.2 When the feed body is empty, fall back to the detail page's `JobPosting` JSON-LD `description` via `GetHTML` + `ldJobPosting` (non-fatal on fetch failure); set `Remote` via the shared `isRemote(location)` heuristic, leave `WorkMode` empty.

## 3. Registration + config

- [x] 3.1 Register `NewEarcu(c)` in `sources.All` (`internal/sources/source.go`) among the board-based ATS adapters.
- [x] 3.2 Confirm config validation requires a board for `earcu` (it is not boardless) and that `earcu` appears in `FilterableProviders()`.

## 4. Tests

- [x] 4.1 `internal/sources/earcu_test.go`: table-driven `Fetch` test over a fixture RSS document (via `fakeHTTP`) asserting title cleanup, URL, ExternalID, location, posted-at, and inline body.
- [x] 4.2 Test the detail-page JSON-LD body fallback (empty feed body → JSON-LD description) and the drop-on-missing-id and non-fatal-detail-failure paths.
- [x] 4.3 `TestEarcuRegisteredInAll` asserting the provider key resolves in `sources.All` (mirror the other `*RegisteredInAll` tests).

## 5. Board file + live validation

- [x] 5.1 Add `sources/earcu.yml` with the header comment and the verified `careers.cambridge.org` entry (company: Cambridge University Press & Assessment).
- [x] 5.2 Probe candidate eArcu hosts from the documented roster live (`/jobs/rss` returns items + carries the `eArcu` marker) and add every host that validates; log any dropped for not validating.
- [x] 5.3 Run `go build ./... && go vet ./... && go test ./internal/sources/` and a dry `cmd/ingest sources/earcu.yml` config-validation pass to confirm the board file loads.
