# design-system-verification Specification

## Purpose
TBD - created by archiving change extend-ds-verification-to-web. Update Purpose after archive.
## Requirements
### Requirement: Primitive adoption is counted and held

The verification suite SHALL report, for every primitive the design system exports, the
number of files under `web/src` that import it, and SHALL compare that report against a
committed baseline. A count that differs from its baseline in either direction SHALL fail.

Adoption is measured in files, not occurrences: a file that imports a primitive counts once
however many times it uses it.

A surface that is a centred modal SHALL use the `Dialog` primitive rather than assembling an
overlay, a focus trap, an Escape handler and a stacking order of its own. A surface that is
not a centred modal — a banner, a drawer, a responsive sheet — SHALL NOT be forced onto it.

#### Scenario: A primitive loses its last consumer

- **WHEN** the only file importing `Skeleton` from `$lib/ui` stops importing it
- **THEN** the check fails, naming `Skeleton` and the drop from its baseline

#### Scenario: A primitive gains consumers

- **WHEN** three files begin importing `Dialog` and the baseline records zero
- **THEN** the check fails, reporting the baseline as stale and naming the flag that rewrites it

#### Scenario: The baseline is rewritten deliberately

- **WHEN** the check runs with its update flag after `Dialog` reached three files
- **THEN** the baseline file records three for `Dialog` and the check passes on the next run

#### Scenario: An unused primitive is named on every run

- **WHEN** the check runs and any exported primitive has no importing file
- **THEN** the report names that primitive as unused, whether or not the run fails

#### Scenario: A modal surface is built by hand

- **WHEN** a centred modal is assembled from `fixed inset-0` and a hand-written Escape handler
- **THEN** it is a `Dialog` call site instead, and the platform provides the top layer, the
  focus trap, Escape and focus restore

#### Scenario: A sheet is not a dialog

- **WHEN** a surface is full-height, edge-anchored, or stretches on mobile and centres above `sm`
- **THEN** it stays as it is, and the absence of a Sheet primitive is recorded rather than
  worked around

### Requirement: The app reaches the design system through one door

App code SHALL import primitives, helpers and types only from `$lib/ui`. A `.svelte` or `.ts`
file under `web/src` that names the `freehire-design-system` package in an import statement
SHALL fail the check unconditionally, with no baseline and no exception.

Stylesheets are excluded: `web/src/app.css` imports the package's `theme.css` directly, and
that import is the package's CSS contract.

#### Scenario: A component imports the package directly

- **WHEN** a Svelte file under `web/src` imports `Button` from `freehire-design-system`
- **THEN** the check fails, naming the file, regardless of any baseline

#### Scenario: The stylesheet imports the theme

- **WHEN** `web/src/app.css` imports `freehire-design-system/theme.css`
- **THEN** the check passes, because the walk reads only JavaScript and Svelte imports

### Requirement: Token discipline covers both sides of the package boundary

The token check SHALL run over `design-system/src` and over `web/src`, and SHALL apply a
different standard to each.

Within `design-system/src` a colour literal or a Tailwind arbitrary value SHALL fail
outright, subject only to the script's declared exception list.

Within `web/src` a colour literal, a Tailwind arbitrary value, or a Tailwind utility built on
the framework's own colour palette SHALL be counted and compared against a committed
baseline, because the existing occurrences cannot be removed in one change and a rule that
cannot be satisfied is a rule that gets switched off.

Both sides SHALL be served by one set of detectors. A detector SHALL NOT be defined twice.

#### Scenario: A literal lands in a primitive

- **WHEN** a hex colour is added to a file in `design-system/src`
- **THEN** the check fails immediately, with no baseline consulted

#### Scenario: A palette utility lands in the app

- **WHEN** a file under `web/src` gains a `text-amber-600` and the baseline is unchanged
- **THEN** the check fails, naming the file, the line, and the utility

#### Scenario: The app sheds a violation

- **WHEN** a `web/src` file replaces a colour literal with a token and the count drops below
  its baseline
- **THEN** the check fails, reporting the baseline as stale and naming the flag that rewrites it

#### Scenario: A palette utility in a primitive

- **WHEN** the palette detector is applied
- **THEN** it runs against `web/src` only, and `design-system/src` is judged by the literal
  and arbitrary-value detectors alone

### Requirement: A ratchet is exact in both directions

Every ratcheted count SHALL fail when the measured value differs from its baseline, whether
the difference is a regression or an improvement. A ratchet SHALL NOT pass silently on an
improvement.

A baseline SHALL be rewritten only by an explicit flag, so that every movement is a
reviewable line in the diff and never a side effect of a passing run.

#### Scenario: An improvement is not silently absorbed

- **WHEN** a ratcheted count falls below its baseline
- **THEN** the check exits non-zero and the baseline file is left unchanged

#### Scenario: A run never writes its own baseline

- **WHEN** the check runs without the update flag
- **THEN** no baseline file is modified, whatever the measured counts

### Requirement: Committed token CSS matches its sources

The compiled stylesheets under `design-system/dist` are committed, and the verification suite
SHALL prove they are the output of the token sources as committed. Rebuilding the tokens and
finding any difference under `dist` SHALL fail.

#### Scenario: A token is edited without a rebuild

- **WHEN** a value in `tokens/*.tokens.json` changes and `dist` is left as it was
- **THEN** the check rebuilds, finds `dist` dirty, and fails

#### Scenario: Sources and output agree

- **WHEN** the tokens are rebuilt from unchanged sources
- **THEN** the output is byte-identical to what is committed and the check passes

### Requirement: The primitives the app depends on carry tests

Every primitive with consuming files under `web/src` SHALL have its consumer-facing contract
pinned by tests: each variant and size resolving to distinct classes, the native attributes a
caller passes through, and a caller-supplied `class` overriding the primitive's own base
classes.

#### Scenario: A variant is dropped

- **WHEN** a variant is removed from a primitive's variant definition
- **THEN** a test fails, rather than the class silently resolving to the default

#### Scenario: A caller overrides a base class

- **WHEN** a call site passes a `class` that collides with one of the primitive's own utilities
- **THEN** the caller's class is the one that applies, and a test asserts it

