## 1. Bare administrator/technician/tester/developer aliases

- [x] 1.1 Add failing tests in `internal/dict/classify/classify_test.go` for
      `Systemadministrator (m/w/d)`, `IT-Systemadministrator (m/w/d)`, `IT
      Systemadministrator (m/w/d)` → `devops`; `Netzwerkadministrator (m/w/d)`
      → `network_engineering`; `Datenbankadministrator (m/w/d)` → `devops`
- [x] 1.2 Add failing tests for `Netzwerktechniker (m/w/d)`, `IT-Netzwerktechniker
      (m/w/d)` → `network_engineering`; `Softwaretester (m/w/d)` → `qa`;
      `Anwendungsentwickler (m/w/d)`, `Inhouse Anwendungsentwickler (m/w/d)` →
      `software_engineering`
- [x] 1.3 Add the six bare aliases to `internal/dict/classify/dictionaries.go`
      alongside the existing German `entwickler` cluster / their English
      counterparts, with a comment recording why each is safe bare (per
      design.md - Decisions)
- [x] 1.4 Run `go test ./internal/dict/classify/...` and confirm the new
      tests pass and no existing test regresses

## 2. Qualified-only Systemtechniker / Systemelektroniker

- [x] 2.1 Add failing tests for `IT Systemtechniker (m/w/d)`,
      `IT-Systemtechniker (m/w/d)`, `IT Systemelektroniker (m/w/d)`,
      `IT-Systemelektroniker (m/w/d)` → `devops`
- [x] 2.2 Add a failing regression test asserting `Systemtechniker (m/w/d)
      Elektrotechnik` and `Systemtechniker Sicherheitstechnik (m/w/d)` resolve
      to NO category
- [x] 2.3 Add the four qualified-only aliases (hyphen and space forms) to
      `dictionaries.go`, with a comment naming the non-IT lookalikes that
      keep the bare word out (per design.md - Decisions and
      `it-title-coverage`'s existing "Systems Engineer" precedent)
- [x] 2.4 Run `go test ./internal/dict/classify/...` and confirm green

## 3. Fachinformatiker family

- [x] 3.1 Add failing tests for bare `Fachinformatiker (m/w/d)` →
      `devops`, `Fachinformatiker Systemintegration (m/w/d)` /
      `Fachinformatiker für Systemintegration (m/w/d)` → `devops`,
      `Fachinformatiker Anwendungsentwicklung (m/w/d)` / `Fachinformatiker für
      Anwendungsentwicklung (m/w/d)` → `software_engineering`
- [x] 3.2 Add the Fachinformatiker aliases to `dictionaries.go` — the two
      qualified phrase pairs declared BEFORE the bare fallback, per the
      file's existing "more specific alias first" ordering rule
- [x] 3.3 Run `go test ./internal/dict/classify/...` and confirm green

## 4. Regression guard for the explicit non-goal

- [x] 4.1 Add (or confirm existing coverage for) a test asserting
      `SPS-Programmierer (m/w/d)` still resolves to NO category, unchanged
      by this change

## 5. Finish

- [x] 5.1 Run the `simplify` skill over the diff — diff already matches file
      conventions closely; no changes needed
- [x] 5.2 Run `gofmt -l .`, `go vet ./...`, `go test ./...` and confirm clean —
      clean except a pre-existing, unrelated failure in
      `cmd/billing-sync.TestTheStoreProviderAloneKeepsTheWorkerRunning`
      (fails identically on an unmodified checkout; outside this change's
      diff and scope)
- [x] 5.3 Request and act on one review pass over the whole diff — reviewer
      verified the boundary-matching/ordering claims against the real
      `wordmatch`/`classify` code and found no functional issues; two
      procedural fixes applied (this checklist, design.md's placement
      description)
