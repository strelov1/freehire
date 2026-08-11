## ADDED Requirements

### Requirement: The Experience UI can update project employment metadata

The web Experience bank SHALL allow the owner to edit a project employment's display name, link, and date fields (and create a project employment) using the existing authenticated employment write APIs, without requiring a CV re-upload or an assistant turn. Job employments MAY gain the same metadata editor; project editing is required for this change.

#### Scenario: Owner corrects a project link in the UI

- **WHEN** a signed-in user edits a `kind=project` employment's link and saves
- **THEN** the bank stores the new link and subsequent list reads return it

#### Scenario: Owner adds a project without uploading a CV

- **WHEN** a signed-in user creates a project employment with a name and optional link via the Experience UI
- **THEN** the employment is persisted and available to CV seed as a project
