## 1. Generator (`cmd/gen-cities`)

- [x] 1.1 Add `cmd/gen-cities/main.go`: download `cities15000.zip` from GeoNames, parse the fixed-column dump into records (name, asciiname, alternatenames, country code, population).
- [x] 1.2 Build the alias set per place (lowercased name + asciiname + native + alternatenames), dedupe; restrict to Latin/Cyrillic scripts; drop short uppercase codes; emit population-sorted so the loader's first-wins picks the most-populous for a shared alias.
- [x] 1.3 Apply the collision stoplist (work-mode / open-anywhere / macro-region markers, mirroring internal/location) to drop unsafe aliases.
- [x] 1.4 Emit a sorted, committed TSV (`canonical <TAB> country <TAB> alias|alias|…`) into `internal/location/`; add a `make gen-cities` target and a generated-file header note.

## 2. Embedded dictionary in `internal/location`

- [x] 2.1 `go:embed` the TSV and parse it once at init (streaming scan, CRLF-tolerant) into the city lookup (alias → canonical name + country code).
- [x] 2.2 Add a small curated-override map (`cityOverrides`) for spellings GeoNames differs on (Köln→Cologne, Zürich→Zurich, …); apply overrides over the generated base (override wins).
- [x] 2.3 Remove the old hand-curated `nameToCity` map (replaced by the generated dictionary + overrides). Keep `nameToCountry` — it stays the authoritative country source and the agreement-guard input; its city entries are a harmless sub-15k fallback.

## 3. Parser resolution

- [x] 3.1 In `location.Parse`, resolve a city token against the merged dictionary and emit the canonical city NAME only — country/region stay the curated dictionaries' job (never guessed from an ambiguous city). When a curated token fixed the country, emit the city name only on per-token country agreement (order-independent).
- [x] 3.2 Verify cooperation with existing separator tokenization, work-mode stripping, Russian city-marker stripping, and dash-export handling (embedded-country and `г <city>` forms still resolve).

## 4. Tests

- [x] 4.1 Table-driven `location` tests: `Florianópolis` → city + br + latam (curated country); `Recife` (long-tail) → city only, no guessed country; `Recife, Brazil` → br; `USA` → us with no stray city; unresolved place emits nothing.
- [x] 4.2 Generator tests (`cmd/gen-cities`): normalize/keepAlias/buildAliases (script + code + stoplist filtering); loader first-wins + override precedence.
- [x] 4.3 Update existing `location` expectations for the now-broader city facet; confirm `clinch` slug splitting is unchanged (net-zero diff — the dictionary adds no geography clinch reads).

## 5. Rollout

- [x] 5.1 `go build ./... && go vet ./... && go test ./...` green.
- [x] 5.2 Document the re-derive + reindex procedure for existing jobs (backfill-derive → reindex), consistent with the dictionary-change convention.
