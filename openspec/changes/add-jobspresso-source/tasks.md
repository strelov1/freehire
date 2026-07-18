## 1. Item mapping (single open posting)

- [ ] 1.1 Add `internal/sources/jobspresso_test.go` with a saved real feed excerpt fixture under
  `testdata/` and a RED test asserting the adapter maps one `<item>` to a `Job` with the right
  fields: `guid` `p=` → `ExternalID`, `<link>` → `URL`, `<title>` → `Title`, `dc:creator`
  `Company<br>⚲&nbsp;Location` → company + location, `content:encoded` → sanitized description,
  `Remote: true` + work mode `remote`, `pubDate` → `PostedAt`.
- [ ] 1.2 Create `internal/sources/jobspresso.go`: a `jobspresso` struct over the RSS
  (`XMLGetter`) client, a `jobspressoItem` struct decoding `title`, `link`, `guid`, `creator`
  (dc:creator local name), `encoded` (content:encoded local name), `description`, and `pubDate`,
  and a `toJob` mapper reusing `sanitizeHTML`, `html.UnescapeString`, and `parsePubDate`. Make
  1.1 green.

## 2. Field-extraction rules

- [ ] 2.1 RED test: `guid` with no `p=` falls back to the `<link>` slug; `content:encoded` empty
  falls back to `<description>`; `dc:creator` with no `<br>` yields company-only + empty location;
  the `⚲` marker and `&nbsp;` are stripped from the location.
- [ ] 2.2 Implement `jobspressoID` (parse `p=`, fallback to link slug) and the `dc:creator` split
  helper; wire the `content:encoded`/`description` preference. Make 2.1 green.

## 3. Drop rules

- [ ] 3.1 RED test: an item with neither `p=` nor a link slug is dropped; an item whose company
  resolves empty is dropped; the remaining items still map.
- [ ] 3.2 Implement the `toJob` drop rules (return ok=false on empty id or empty company). Make
  3.1 green.

## 4. Classification & registration

- [ ] 4.1 Add the `boardless()` and `aggregator()` markers and `Provider()` returning
  `"jobspresso"`; register `jobspresso` in `sources.All` (one line, under the multi-company
  aggregators). Test the provider resolves from `All(nil)`. Verify `go build ./... && go vet ./...`.

## 5. Board file & docs

- [ ] 5.1 Add `sources/jobspresso.yml` with a single boardless entry (`company: Jobspresso`) and
  confirm `go run ./cmd/ingest sources/jobspresso.yml` validates against the registry (fail-fast
  passes).
- [ ] 5.2 Note the new adapter in `internal/sources/AGENTS.md` if it enumerates adapters by class
  (keyless RSS feed), keeping the surrounding style.

## 6. Verify end-to-end

- [ ] 6.1 Run `go test ./internal/sources/...` (all green), then a throwaway live smoke crawl
  against the real feed to confirm items map and unusable ones are dropped without error (not
  committed).
