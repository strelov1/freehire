## 1. Derive remote/work-mode from homeofficemoeglich

- [x] 1.1 Update `internal/sources/arbeitsagentur_test.go`: extend the detail fixture(s) with
  `jobdetail.homeofficemoeglich` and assert a `true` posting maps to `Remote: true` + work mode
  `remote`, a `false`/absent posting leaves both unset, and a failed/empty detail still emits the
  posting with no remote flag (RED).
- [x] 1.2 Update `internal/sources/arbeitsagentur.go`: add `Homeofficemoeglich bool` to the detail
  struct, have the detail parse return it, and set `Remote` + `WorkMode` in `toJob` via
  `workModeFromRemote`. Make 1.1 green.

## 2. Verify

- [x] 2.1 `gofmt`, `go build ./... && go vet ./...`, `go test ./internal/sources/` green; then a
  throwaway live check that a real `homeofficemoeglich: true` arbeitsagentur posting maps to remote
  (not committed).
