## 1. Adapter

- [x] 1.1 Write table-driven `tecla_test.go` on a fake `HTTPClient`: assert pagination across `countPages`, per-job company from the payload, `ExternalID`/`URL`/`Title`/`PostedAt`/`Description` mapping, `Remote`/`WorkMode="remote"`, and the boardless marker (RED)
- [x] 1.2 Implement `internal/sources/tecla.go`: `NewTecla`, `Provider()`, `boardless()`, `Fetch` paginating `getPublicJobs` over `countPages` (capped), typed JSON decode, posting→`Job` mapping with the no-timezone `createdAt` layout (GREEN)

## 2. Wiring

- [x] 2.1 Register `NewTecla(c)` in `sources.All` (`internal/sources/source.go`)
- [x] 2.2 Add `sources/tecla.yml` with one boardless entry (`company: Tecla`, `provider: tecla`)

## 3. Verify

- [x] 3.1 `go build ./...`, `go vet ./...`, `go test ./internal/sources/` green
- [x] 3.2 Live smoke against the real API: `go run ./cmd/ingest sources/tecla.yml` (or an equivalent fetch) returns real postings with correct per-job company and remote flag
