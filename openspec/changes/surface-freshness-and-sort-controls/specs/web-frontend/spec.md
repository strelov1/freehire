## REMOVED Requirements

### Requirement: Jobs browse sort control

**Reason**: The requirement describes a **Date posted** / **Recently added** pair
that the client has not offered for some time — the select's vocabulary became
`Newest` / `Best match`, and the requirement was never revised, so it documented
a control that does not exist. `jobs-list-controls` supersedes it with the sort
control's actual vocabulary, its contextual default, and its visibility rule.

**Migration**: None. `created_at` remains an accepted, sortable attribute of the
search endpoint (`job-search`), so an API integration carrying `sort=created_at`
is unaffected.

A **browse-UI link** carrying it is a different matter, and was so before this
change: the client resolved any unrecognised `sort` — `created_at` among them —
to its default and dropped the param on the next serialization. That is why the
requirement being removed here was already describing a control the code did not
have. This change does not alter that behaviour, and does not undertake to
preserve it.
