## 1. Generator (`cmd/gen-cities`)

- [ ] 1.1 Add `cmd/gen-cities/main.go`: download `cities15000.zip` from GeoNames, parse the fixed-column dump into records (name, asciiname, alternatenames, country code, population).
- [ ] 1.2 Build the alias set per place (lowercased name + asciiname + native + alternatenames), dedupe; keep the most-populous place for any bare name shared across places.
- [ ] 1.3 Apply the collision stoplist (curated common words + parser work-mode/open-anywhere markers) to drop unsafe aliases.
- [ ] 1.4 Emit a sorted, committed TSV (`canonical <TAB> country <TAB> alias|alias|…`) into `internal/location/`; add a `make gen-cities` target and a generated-file header note.

## 2. Embedded dictionary in `internal/location`

- [ ] 2.1 `go:embed` the TSV and parse it once at init into the city lookup (alias → canonical name + country code).
- [ ] 2.2 Add a small curated-override map for cities GeoNames lacks / spells differently (e.g. `Cupertino`, ATS shorthands); apply overrides over the generated base (override wins).
- [ ] 2.3 Remove the now-redundant hand-curated city entries from `nameToCity`/`nameToCountry` that the generated source covers, keeping only genuine overrides.

## 3. Parser resolution

- [ ] 3.1 In `location.Parse`, resolve a city token against the merged dictionary so a hit writes BOTH the canonical city name and the country code (→ region) from one lookup.
- [ ] 3.2 Verify cooperation with existing separator tokenization, work-mode stripping, Russian city-marker stripping, and dash-export handling (embedded-country and `г <city>` forms still resolve).

## 4. Tests

- [ ] 4.1 Table-driven `location` tests: `Florianópolis` → city `Florianópolis` + country `br` + region `latam`; `Florianópolis, Brazil` and `г Москва` forms; an unresolved place emits nothing.
- [ ] 4.2 Stoplist tests: a stoplisted common word / work-mode word resolves no city.
- [ ] 4.3 Override tests: a curated alias GeoNames lacks (`Cupertino`) still resolves.

## 5. Rollout

- [ ] 5.1 `go build ./... && go vet ./... && go test ./...` green; regenerate contracts if the location surface exported to the web changed (`make gen-contracts`).
- [ ] 5.2 Document the re-derive + reindex procedure for existing jobs in the change (backfill-derive → reindex), consistent with the dictionary-change convention.
