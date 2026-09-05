## ADDED Requirements

### Requirement: The AI filter is entered from the filter modal at every viewport width

The "describe your filters in words" entry point SHALL be rendered inside the filters
modal, and SHALL NOT be rendered in the desktop filters sidebar.

The sidebar is hidden below the `md` breakpoint, so an entry point that lives only there
does not exist on a phone. The modal opens at every width, so hosting the entry there both
frees the sidebar column and gives the feature to mobile.

The entry point SHALL remain visible to signed-out visitors, with activation prompting
sign-in — hiding it from them would hide the feature from exactly the people who have not
yet been given a reason to sign in.

#### Scenario: A phone reaches the AI filter

- **WHEN** a visitor on a sub-`md` viewport opens the filters modal
- **THEN** the AI filter entry point is present and activates the same dialog desktop uses

#### Scenario: The sidebar no longer carries it

- **WHEN** a visitor on a desktop viewport views the filters sidebar
- **THEN** the AI filter entry point is not there, and the column it occupied is free

#### Scenario: A signed-out visitor is prompted rather than hidden from

- **WHEN** a signed-out visitor activates the AI filter entry point
- **THEN** the sign-in prompt opens, rather than the entry point having been absent
