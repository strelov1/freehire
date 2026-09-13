## ADDED Requirements

### Requirement: The modal offers an Auto-apply available checkbox

The filter modal SHALL offer an "Auto-apply available" checkbox alongside the
existing bespoke boolean checkboxes ("Offers visa sponsorship", "Hide
employers reported to interview with AI"), staged and applied the same way as
those. It SHALL NOT be rendered through the generic facet-rail system, which
deliberately never exposes raw `source` to candidates.

Turning it on SHALL stage the `auto_apply_available=true` filter; turning it
off SHALL remove the filter entirely (never `auto_apply_available=false`).

The checkbox SHALL carry a caption making clear the signal is best-effort: a
posting matching the filter is not a guaranteed successful submission.

#### Scenario: The checkbox sits with the other bespoke checkboxes

- **WHEN** the filter modal is open
- **THEN** "Auto-apply available" is offered alongside "Offers visa
  sponsorship" and "Hide employers reported to interview with AI"

#### Scenario: Turning the checkbox on stages the filter

- **WHEN** a candidate checks "Auto-apply available" and applies the modal
- **THEN** the job list filters to postings marked `auto_apply_available`

#### Scenario: Turning the checkbox off clears the filter

- **WHEN** a candidate unchecks a previously-applied "Auto-apply available"
  and applies the modal
- **THEN** the job list is no longer filtered on `auto_apply_available`

#### Scenario: The checkbox shows a best-effort caption

- **WHEN** the filter modal renders the "Auto-apply available" checkbox
- **THEN** it shows a caption stating that a matching posting is not a
  guaranteed successful submission
