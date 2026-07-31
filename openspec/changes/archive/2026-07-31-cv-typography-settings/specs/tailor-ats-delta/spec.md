## MODIFIED Requirements

### Requirement: The comparison holds everything but the tailoring constant

The delta SHALL compare the tailored CV against the base CV it was copied from, with every scoring
input other than the document content held identical: the base CV SHALL be rendered with the
**tailored copy's** template, page margins, and typography, and both sides SHALL be scored against
the **same** keyword baseline — the canonical skills of the vacancy the tailored CV is bound to.

The base CV's own template, margins, and typography SHALL NOT be used, and the base CV SHALL NOT be
modified in any way by scoring it.

Typography belongs in that list for the same reason template and margins do: type size and leading
change how much text lands on a page and therefore what the rendered text layer contains. Leaving it
out would let a candidate move their own ATS delta by changing a font, with the content untouched.

#### Scenario: A template difference does not leak into the delta
- **WHEN** the base CV's stored template differs from the tailored copy's
- **THEN** both sides are rendered with the tailored copy's template, so the delta reflects content
  only

#### Scenario: A typography difference does not leak into the delta
- **WHEN** the base CV's stored typography differs from the tailored copy's
- **THEN** both sides are rendered with the tailored copy's font, size, and line height, so the delta
  reflects content only

#### Scenario: Both sides share one keyword baseline
- **WHEN** the delta is computed for a CV bound to a vacancy
- **THEN** the vacancy's canonical skills are the keyword baseline for both the base and the
  tailored score

#### Scenario: Scoring the base CV leaves it untouched
- **WHEN** the delta is computed
- **THEN** the base CV's stored document, template, margins, and typography are unchanged afterwards
