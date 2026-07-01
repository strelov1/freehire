## 1. Fix the Sber apply URL

- [ ] 1.1 Add a failing test in `internal/sources/sber_test.go` asserting that a `sber`
  posting whose feed record has `requisitionId` GUID and numeric `internalId` yields a job
  with `URL == "https://rabota.sber.ru/search/<internalId>"` and `ExternalID == <requisitionId>`.
- [ ] 1.2 Add `InternalID int64` (json `internalId`) to `sberVac` and build the job `URL`
  from it (`sberVacURL` formatted with `%d`); keep `ExternalID` on `RequisitionID`.
- [ ] 1.3 Run `go test ./internal/sources/` and `go vet ./...` — green.
