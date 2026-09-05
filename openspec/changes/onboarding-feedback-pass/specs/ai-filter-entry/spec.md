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

It SHALL appear only where the modal edits a LIVE filter store — every surface that
carried it in the sidebar, and no more. Two modals are therefore without it, both
correctly: the profile editor, where a facet value is a plain choice rather than a search,
and the homepage, which composes a query to navigate with instead of narrowing a list in
front of the visitor. Giving the homepage one would mean inventing a second way for the
interpretation to be applied; that is a feature, not this change, and it was not in the
sidebar either.

#### Scenario: A phone reaches the AI filter

- **WHEN** a visitor on a sub-`md` viewport opens the filters modal
- **THEN** the AI filter entry point is present and activates the same dialog desktop uses

#### Scenario: The sidebar no longer carries it

- **WHEN** a visitor on a desktop viewport views the filters sidebar
- **THEN** the AI filter entry point is not there, and the column it occupied is free

#### Scenario: A signed-out visitor is prompted rather than hidden from

- **WHEN** a signed-out visitor activates the AI filter entry point
- **THEN** the sign-in prompt opens, rather than the entry point having been absent
