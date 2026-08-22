## ADDED Requirements

### Requirement: Every internal package belongs to exactly one block

Every package under `internal/` SHALL live at `internal/<block>/<pkg>`, where `<block>` is
one of exactly eleven names: `platform`, `dict`, `ai`, `identity`, `candidate`, `job`,
`application`, `search`, `ingest`, `engage`, `api`. No package SHALL sit directly under
`internal/`, and no twelfth block SHALL be introduced without amending this specification.

#### Scenario: A package sits directly under internal/

- **WHEN** a package exists at `internal/<pkg>` with no block segment
- **THEN** the layering check fails, naming the unassigned package

#### Scenario: A package sits under an unrecognized block

- **WHEN** a package exists at `internal/<block>/<pkg>` and `<block>` is not one of the eleven
- **THEN** the layering check fails, naming the unrecognized block

### Requirement: A block imports only blocks strictly below it

The blocks SHALL form eight totally ordered layers:

| Layer | Blocks |
|---|---|
| 8 | `api` |
| 7 | `engage`, `ingest` |
| 6 | `application`, `search` |
| 5 | `job` |
| 4 | `candidate` |
| 3 | `ai`, `identity` |
| 2 | `dict` |
| 1 | `platform` |

A package in a block at layer N SHALL import packages only from blocks at layers strictly
less than N, or from its own block. Imports between two blocks sharing a layer are
forbidden in both directions, which is what keeps the layer total rather than partial.

#### Scenario: An import points at a higher layer

- **WHEN** a package in `internal/dict` (layer 2) imports a package in `internal/job` (layer 5)
- **THEN** the layering check fails, reporting the importing package, the imported package, and both layers

#### Scenario: An import points at the same layer, different block

- **WHEN** a package in `internal/engage` imports a package in `internal/ingest` (both layer 7)
- **THEN** the layering check fails

#### Scenario: An import points at a lower layer

- **WHEN** a package in `internal/api` (layer 8) imports a package in `internal/platform` (layer 1)
- **THEN** the layering check passes

#### Scenario: An import stays inside its own block

- **WHEN** `internal/ingest/pipeline` imports `internal/ingest/sources`
- **THEN** the layering check passes

### Requirement: The rule is enforced at two granularities

The layering rule SHALL be enforced by `depguard` in `.golangci.yml`, which fails at the
offending import line, AND by a Go test that evaluates the whole import graph in one pass
and reports every violation together. The test SHALL cover test-only imports
(`TestImports`, `XTestImports`) as well as production imports, because a test file can
create a dependency the production build never reveals.

#### Scenario: A violating import is added in a pull request

- **WHEN** a pull request adds an import that crosses a layer upward
- **THEN** `golangci-lint` fails on that line, and the layering test fails naming the pair

#### Scenario: A violation exists only in a test file

- **WHEN** a `_test.go` file in a lower block imports a package from a higher block
- **THEN** the layering test fails

### Requirement: Both guards SHALL see the build-tagged files

The repo carries 222 files behind `//go:build integration` and 2 behind `//go:build
llmlive`, and `internal/db`'s tagged tests are in-package (`package db`), so their imports
constrain `db` itself. Both the layering test's `go list` invocation and `golangci-lint`
SHALL be configured with those tags. An untagged guard passes over exactly the case
`CLAUDE.md` warns about: green in every local command except the tagged `vet`, then a
failure in CI, which runs `go test -tags=integration ./...` over the whole module.

Adding the tags is sound rather than merely convenient: the repo has no negated build
constraint and no legacy `// +build` line, so the tagged build is a strict superset of the
untagged one and no file drops out.

#### Scenario: A cross-layer import exists only in an integration-tagged test

- **WHEN** an in-package file behind `//go:build integration` imports a package from a higher block
- **THEN** the layering test fails, and `golangci-lint` reports the import line

#### Scenario: The tag list omits a constraint the repo uses

- **WHEN** a new build constraint is introduced without being added to the guard's tag list
- **THEN** the files behind it are invisible to both guards, so the tag list is maintained alongside any new constraint

### Requirement: Module guards that locate their target by string path SHALL resolve

Four existing guards find their target by string path rather than by import, so a stale
path makes them pass over nothing instead of failing: `internal/llmkey/scope_test.go` (the
guard that background entrypoints never resolve a user's LLM credential),
`internal/normalize/legal_form_rule_test.go` (one legal-form vocabulary per module),
`internal/pgerr/pgerr_test.go`, and `cmd/gen-cities/main.go`'s `outputPath`. Each SHALL
name a path that exists after the move, and each SHALL fail when pointed at a path that
does not.

#### Scenario: A path guard is pointed at a nonexistent path

- **WHEN** any of the four guards is deliberately given a path that does not exist
- **THEN** it fails rather than passing vacuously

#### Scenario: The LLM credential guard still sees the background entrypoints

- **WHEN** the scope guard runs after the move
- **THEN** it resolves `enrich`, `telegram`, `mailclassify`, and `embed` at their new block paths and still asserts none of them resolves a user credential

### Requirement: The move changes no behaviour

The change SHALL be import-path-only plus four mechanical extractions. No requirement in
any other capability changes, nothing on the wire moves, and no database schema is touched.

#### Scenario: The full test suite after the move

- **WHEN** `go test ./...` and `go test -tags=integration ./...` run after the move
- **THEN** they pass with no test's assertions altered
