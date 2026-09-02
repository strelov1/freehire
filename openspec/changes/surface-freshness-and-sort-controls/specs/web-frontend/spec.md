## REMOVED Requirements

### Requirement: Jobs browse sort control

**Reason**: The requirement describes a **Date posted** / **Recently added** pair
that the client has not offered for some time — the select's vocabulary became
`Newest` / `Best match`, and the requirement was never revised, so it documented
a control that does not exist. `jobs-list-controls` supersedes it with the sort
control's actual vocabulary, its contextual default, and its visibility rule.

**Migration**: None for callers. `created_at` remains an accepted, sortable
attribute of the search endpoint (`job-search`), so any link or integration
carrying `sort=created_at` keeps working; it is simply no longer offered as a
choice in the browse UI.
