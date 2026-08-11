## 1. Location round-trip through PII contacts

- [x] 1.1 Add `Location` to `pii.Contacts`; have `fillContact` record the first non-empty `ADDRESS` span into it
- [x] 1.2 In `resumeextract.Extract`, assign `s.Location` from `red.Contacts()` with the other contact fields (detection wins over any model value)
- [x] 1.3 Adjust the extraction system prompt / schema guidance so location is not framed as a field the model must invent from redacted text (contacts-from-detection rule)
- [x] 1.4 Tests: `Contacts()` carries location from an ADDRESS span; Extract persists that location while the model prompt stays redacted; no address span → empty location

## 2. Seed usability gate

- [x] 2.1 Change `bankedSeeder.Structured` so the usable bool requires a current structured résumé (`ok` from the résumé reader) **and** the composed value still passes `seedable` — bank-only composition returns usable=false
- [x] 2.2 Cover with unit tests: structure present + bank → usable; structure absent/stale + bank experience → not usable; structure present but empty of every seeded field → not usable

## 3. Header merge on apply

- [x] 3.1 Update `applySeedContent` so empty seed header fields (`full_name`, `email`, `phone`, `location`, empty `links`) preserve the keep document's values; non-empty seed values still replace
- [x] 3.2 Tests: empty seed contacts leave a filled header intact; non-empty seed phone replaces; body still comes from the seed

## 4. Bootstrap / reset paths honour the gate

- [x] 4.1 Confirm `reseedBaseIfStaleVsUpload` and `ResetCVFromResume` refuse (no write) when the seed is not usable — existing 409 paths, no blank-header document
- [x] 4.2 Add / extend handler tests for: stale upload + pending structure does not wipe base header on tailor; reset-from-résumé with bank-only seed is 409 and leaves the tailored header unchanged

## 5. Verify

- [x] 5.1 `go test ./internal/pii/ ./internal/resumeextract/ ./internal/handler/ -count=1` (and any `internal/cv` seed tests touched) green
- [x] 5.2 `go vet -tags=integration ./...` green before push
