## MODIFIED Requirements

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
