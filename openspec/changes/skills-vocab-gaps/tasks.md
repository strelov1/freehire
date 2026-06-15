## 1. Add the missing tech aliases (TDD)

- [x] 1.1 Add failing tests in `internal/skilltag` (Parse over a realistic description): each new skill resolves — `agile`, `scrum`, `kanban`, `salesforce`, `sap`, `oracle`, `devops`, `microservices`, `observability`, `rest` (via "RESTful APIs" / "REST API"), `powerbi` (via "Power BI"); and a trap: a description with "the rest of the team" (and none of the real phrases) does NOT tag `rest`.
- [x] 1.2 Add the word aliases (`agile`, `scrum`, `kanban`, `salesforce`, `sap`, `oracle`, `devops`, `microservices`, `microservice`, `observability`, `restful`) to `wordAliases` and the phrase aliases (`rest api`/`rest apis` → `rest`, `power bi`/`power-bi` → `powerbi`) to `phraseAliases` in `dictionaries.go`. No engine change.
- [x] 1.3 Run `go test ./internal/skilltag/` green (incl. the existing canonical-slug invariant test).

## 2. Verify

- [x] 2.1 `go build ./... && go vet ./... && go test ./...` green; `gofmt -l` clean; confirm no other package regressed.
