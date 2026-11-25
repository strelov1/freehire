## 1. Résumé store — share provisional contacts

- [x] 1.1 Export a résumé-store helper that returns provisional contact fields from a superseded blob when the stamp is not current (reuse `provisionalContacts` / `ProfileReadForUser` contact slice; keep `Structured` stamp-gated)
- [x] 1.2 Unit tests: provisional contacts present when stale; empty when no blob; stamp gate still false for `Structured`

## 2. Seed composition

- [x] 2.1 Update `bankedSeeder` to layer provisional contacts when current structure is absent; treat that as enough for the structure side of usable when contacts are non-empty; never copy superseded semantic sections
- [x] 2.2 Update seed unit tests: provisional + bank usable with header filled; bank-only still unusable; stale semantic fields stay out of seed
- [x] 2.3 Adjust stale-base refresh / reset integration expectations so provisional + bank can refresh an empty header instead of no-op/refuse

## 3. Empty-header heal on open

- [x] 3.1 On tailored CV owner load (GET and tailor bootstrap returning an existing copy), merge provisional contacts into empty header fields via `mergeSeedHeader`, skip write when unchanged, persist when healed
- [x] 3.2 Heal empty base header under the same conditions so the next tailor copy is not blank
- [x] 3.3 Tests: empty tailored header heals on open; partial header preserves non-empty fields; no provisional → no write

## 4. Verify

- [x] 4.1 `go test` for touched packages + `go vet -tags=integration ./...`
