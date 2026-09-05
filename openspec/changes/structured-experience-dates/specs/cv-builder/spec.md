## ADDED Requirements

### Requirement: A CV document's period dates are structured, not free text
An experience or education entry's start/end period within a CV document SHALL
be a structured year/(optional month) value plus a boolean marking an ongoing
entry, matching the same representation used by the structured résumé and the
experience bank — not a free-form string. Rendering a CV to PDF SHALL format
this structured value into display text (e.g. "Mar 2021", "2018", "Mar 2021 –
Present") before it reaches the template layer, so no template needs to know
the underlying value is structured. This display text is canonical — one
fixed style regardless of how the original free text was worded — so an
entry whose period was previously stored as free text in a different style
(e.g. "October 2018") SHALL render in the canonical style ("Oct 2018") after
this change, not its original wording; the boundary's year and month are
unchanged, only the display style is.

#### Scenario: A value already in canonical style renders unchanged
- **WHEN** a CV document has an experience entry with a structured start of
  {year: 2018, month: 3} and no end (current)
- **THEN** the rendered PDF shows "Mar 2018 – Present" — the same text a
  free-text field already written in that exact style would have rendered

#### Scenario: A pre-existing entry's display text is canonicalized, not preserved verbatim
- **WHEN** an entry's period was originally stored as free text in a style
  different from the renderer's canonical format (e.g. "October 2018", where
  the canonical style is "Oct 2018")
- **THEN** the rendered PDF shows the canonical style, not the original
  wording — the boundary's year and month are unchanged, only the display
  style is

#### Scenario: A year-only period renders without a fabricated month
- **WHEN** a CV document has an entry whose period carries only a year
- **THEN** the rendered PDF shows only the year for that boundary, not a
  month the candidate never specified
