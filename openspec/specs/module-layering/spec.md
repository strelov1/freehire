# module-layering Specification

## Purpose
The shape of `internal/`: which block every package belongs to, which blocks may see which,
and how that is enforced. It exists so the directory tree carries the information the root
`AGENTS.md` table used to carry alone, and so an import that crosses a boundary fails CI
rather than merging.
## Requirements
### Requirement: Every internal package belongs to exactly one block

Every package under `internal/` SHALL live at `internal/<block>/<pkg>`, where `<block>` is
one of exactly eleven names: `platform`, `dict`, `ai`, `identity`, `candidate`, `job`,
`application`, `search`, `ingest`, `engage`, `api`. No package SHALL sit directly under
`internal/`, and no twelfth block SHALL be introduced without amending this specification.

The assignment lives in `internal/platform/arch/layering/blocks.go` and is the single
source both the guard test and the generated `depguard` rules read. A package that exists
in the repo but not in that table fails the guard, as does a table entry naming a package
that does not exist.

#### Scenario: A package sits directly under internal/

- **WHEN** a package exists at `internal/<pkg>` with no block segment
- **THEN** the layering check fails, naming the unassigned package

#### Scenario: A package sits under an unrecognized block

- **WHEN** a package exists at `internal/<block>/<pkg>` and `<block>` is not one of the eleven
- **THEN** the layering check fails, naming the unrecognized block

#### Scenario: A new package is added without registering it

- **WHEN** a package is created under a valid block but not added to `blocks.go`
- **THEN** the layering check fails, naming the package

#### Scenario: A package sits in a block other than the one assigned to it

- **WHEN** a package's path names a different block from the one `Assignment` gives it
- **THEN** the layering check fails, naming both. Checking layer compliance alone would not: a misplacement between two blocks on adjacent layers crosses nothing

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
forbidden in both directions, which is what keeps the layer total rather than partial —
two blocks that can see each other are one block under two names.

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
offending import line, AND by a Go test in `internal/platform/arch/layering` that evaluates
the whole import graph in one pass and reports every violation together. The test SHALL
cover test-only imports (`TestImports`, `XTestImports`) as well as production imports,
because a test file can create a dependency the production build never reveals.

The `depguard` rules SHALL be derived from the same layer table the test reads, so the two
cannot disagree.

#### Scenario: A violating import is added in a pull request

- **WHEN** a pull request adds an import that crosses a layer upward
- **THEN** `golangci-lint` fails on that line, and the layering test fails naming the pair

#### Scenario: A violation exists only in a test file

- **WHEN** a `_test.go` file in a lower block imports a package from a higher block
- **THEN** the layering test fails

### Requirement: Both guards SHALL see the build-tagged files

The repo carries 222 files behind `//go:build integration` and 2 behind `//go:build
llmlive`, and many are in-package tests, whose imports constrain the package itself. Both
the layering test's `go list` invocation and `golangci-lint`'s `run.build-tags` SHALL name
those tags. An untagged guard passes over exactly the case `AGENTS.md` warns about: green
in every local command except the tagged vet, then a failure in CI, which runs
`go test -tags=integration ./...` over the whole module.

Naming the tags is sound rather than merely convenient: the repo has no negated build
constraint and no legacy `// +build` line, so the tagged build is a strict superset of the
untagged one and no file drops out.

#### Scenario: A cross-layer import exists only in an integration-tagged test

- **WHEN** an in-package file behind `//go:build integration` imports a package from a higher block
- **THEN** the layering test fails, and `golangci-lint` reports the import line

#### Scenario: The tag list omits a constraint the repo uses

- **WHEN** a new build constraint is introduced without being added to the guard's tag list
- **THEN** the files behind it are invisible to both guards, so the tag list is maintained alongside any new constraint

### Requirement: Module-walking guards SHALL anchor at the module root

Several tests police a rule across the whole repository rather than within their own
package — that only `internal/platform/pgerr` classifies a SQLSTATE, that background
entrypoints never resolve a user's LLM credential, that every pool-opening command shares
one bootstrap, that one legal-form vocabulary exists. Each walks a tree rooted above its
own package, and each SHALL locate that root through `internal/platform/modroot`, never by
counting `../` segments.

A counted depth fails silently in the safe-looking direction: the walk simply covers less
of the repo and the guard keeps passing. Each such guard SHALL additionally fail rather
than pass when its target is missing or its walk is empty.

#### Scenario: A guard's target path does not exist

- **WHEN** a module-walking guard is pointed at a path that does not exist
- **THEN** it fails, rather than walking nothing and passing

#### Scenario: A guarded package moves

- **WHEN** a package carrying such a guard is relocated to a different depth
- **THEN** the guard still walks the same tree, because the root is found by locating `go.mod`
