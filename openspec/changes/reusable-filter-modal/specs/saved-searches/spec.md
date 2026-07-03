## MODIFIED Requirements

### Requirement: Saved-search UI in the filters panel

The web filter modal SHALL present a "My filters" tab to signed-in users for
selecting, saving, updating, and deleting saved searches, and SHALL prompt anonymous
users to sign in instead of showing the list. Because the modal defers, the tab
SHALL operate on the staged filters: selecting a set seeds the staged state and
saving captures the staged filters; the changes reach the live filter state (and the
URL) only when the modal's **Show results** action is activated.

#### Scenario: Apply a saved search

- **WHEN** a signed-in user selects a saved set from the "My filters" tab and activates
  **Show results**
- **THEN** the modal parses the stored query string into the staged filters and, on
  **Show results**, commits it to the URL, and the results re-search accordingly

#### Scenario: Active set is marked

- **WHEN** the staged filter state's canonical query string equals a saved set's query
- **THEN** that set is marked active (checkmark) in the control

#### Scenario: Anonymous prompt

- **WHEN** an anonymous (signed-out) user opens the "My filters" tab
- **THEN** the control shows a "sign in to save" affordance that opens the auth dialog
  instead of a list of sets

## REMOVED Requirements

### Requirement: Share affordance in the filters panel

**Reason**: Board sharing clutters the in-context filters control and duplicates the
dedicated management surface. Share, unshare, and copy-link are removed from the
"My filters" control so it focuses on selecting/saving/updating/deleting.

**Migration**: Users share and unshare a saved search from the account section at
`/my/searches` (the "Saved searches section in the account area" requirement), which
retains the full share/unshare/copy-link management unchanged.
