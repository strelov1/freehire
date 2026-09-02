## 1. Russian software vocabulary

- [x] 1.1 Add `программист`, `разработчик`, `инженер-программист`, `техник-программист` → `software_engineering` as bare tokens, with tests covering the technology-first spellings (`Java-разработчик`, `Python-разработчик`) that only the bare form can reach.
- [x] 1.2 Add `системный администратор` and `администратор баз данных` → `devops`, and `сетевой администратор` → `network_engineering`, with tests.

## 2. The Systems Engineer family

- [x] 2.1 Declare the non-IT lookalikes BLIND with `categoryNone` — `control systems engineer`, `power systems engineer`, `electrical systems engineer`, `quality systems engineer` — placed ABOVE the bare alias, with tests asserting each still resolves to no category.
- [x] 2.2 Add the qualified IT spellings: `linux systems engineer` → `devops`, `cyber systems engineer` → `security`, `software systems engineer` / `it systems engineer` → `software_engineering`, with tests.
- [x] 2.3 Add the bare `systems engineer` / `system engineer` → `software_engineering` BELOW the blind entries, with tests for the numbered spellings ("Systems Engineer II").

## 3. Vendor platforms

- [x] 3.1 Add the ServiceNow family (`servicenow developer`/`engineer` → `software_engineering`, `servicenow administrator` → `devops`) and the Salesforce family beyond Developer (`salesforce administrator`/`engineer`/`consultant` → `software_engineering`), with tests.
- [x] 3.2 Add `oracle dba`, `sharepoint administrator` → `devops`; `mainframe developer` → `software_engineering`; `tableau developer` → `data_analytics`, with tests.

## 4. Infrastructure and support tail

- [x] 4.1 Add the data-centre and operations titles → `devops`: `data center technician`/`engineer`, `release engineer`, `cloud operations engineer`, `cloud migration engineer`, `network operations engineer`, with tests.
- [x] 4.2 Add `network specialist`, `network technician` → `network_engineering` and `it specialist`, `it technician` → `support`, with tests.
- [x] 4.3 Add the integration family (`integration engineer` plus the systems/software/data/cloud qualified spellings) → `software_engineering`, with tests.

## 5. Named roles

- [x] 5.1 Add `salesforce_developer`, `sap_developer`, `servicenow_developer` and `systems_engineer` to `roletag`, ordered so no entry steals a longer alias, with tests.
- [x] 5.2 Fix `systems_administrator` to resolve from the SINGULAR spelling and its variants (`system administrator`, `sysadmin`, `linux system administrator`, `windows system administrator`, `системный администратор`), with tests.

## 6. Collision guards

- [x] 6.1 Add regression tests that "Parts Counterperson", "Parts Interpreter", "Pit Technician" and "SAP Operations Clerk Part Time Day" still resolve to no category, and that "Sales Engineer" and "Support Engineer" keep theirs.

## 7. Verification

- [x] 7.1 Regenerate `web/src/lib/generated/contracts.ts` via `cmd/gen-contracts` and confirm the new role labels are present and no CATEGORY value changed.
- [x] 7.2 Run `gofmt -l .`, `go vet ./...`, `go test ./...`, `go vet -tags=integration ./...`, `golangci-lint run`; all clean.
- [x] 7.3 Re-run the mining pass against the same prod dump and record the new unroled share in the change, so the rollout has a number to be checked against.
