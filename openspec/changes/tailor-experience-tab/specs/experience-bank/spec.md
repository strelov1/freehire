## ADDED Requirements

### Requirement: An unconfirmed atom can be confirmed with one action

The system SHALL offer a single action that confirms an `agent_inferred` atom without requiring
the owner to open or change its claim text — it re-submits the atom's own current claim through
the existing update path, which already re-stamps provenance to `manual` on any call regardless of
whether the text changed. This is a second way to reach the same outcome the existing edit-and-save
flow reaches; editing and confirming both leave the atom as the owner's own statement, usable in a
tailored CV.

#### Scenario: Confirming does not require opening the edit field

- **WHEN** the owner activates the confirm action on an `agent_inferred` atom
- **THEN** the atom's claim text is unchanged and its provenance becomes `manual`, without the
  owner having opened an edit field

#### Scenario: A confirmed atom is publishable

- **WHEN** an atom confirmed through this action is later cited by its id as `evidence_id` on a
  `cv_edit`
- **THEN** the citation is accepted, the same as any other `manual`-provenance atom
