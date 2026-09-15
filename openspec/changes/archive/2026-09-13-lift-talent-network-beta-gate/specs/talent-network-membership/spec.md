## REMOVED Requirements

### Requirement: Joining is limited to the beta group while the feature settles

**Reason**: The gate existed only so a working catalogue would not open onto a beta-sized
population. The feature has had time to accumulate a real membership and is ready to
open to every signed-in candidate; there is no successor gate.

**Migration**: No data migration. Every account, beta or not, may now join and leave
freely — no stored state changes shape, and an existing member's row is untouched. The
catalogue list page no longer carries a forced `noindex` (see
`talent-network-catalog`'s pre-existing "The list is indexable, a card is not"
requirement, which this change now actually satisfies in production rather than only in
spec).

The system SHALL refuse a request to JOIN from an account outside the beta group, and
SHALL enforce that on the server rather than by hiding a control.

LEAVING SHALL never be refused. A gate that also held somebody in would be a gate that
traps them, and the one thing this feature promises is that leaving works — including for
an account that joined while in the group and was later taken out of it.

Hiding the entry points is an affordance, not the gate. The catalogue's data is public by
design, so a client-side check closes nothing; what keeps the catalogue to the beta
population is that nobody outside it can put themselves in.

While the gate stands, the catalogue listing SHALL NOT be offered to search engines. It is
meant to be indexable — it is the front door of the feature and carries no personal data —
but a catalogue whose whole membership is the beta group is not the one worth indexing,
and a search result promising candidates that leads to four is worse than no result.

#### Scenario: An account outside the group tries to join

- **WHEN** an account that is not in the beta group asks to join
- **THEN** the request is refused
- **AND** nothing is stored: no membership, and no handle minted

#### Scenario: An account outside the group leaves

- **WHEN** a member who is no longer in the beta group asks to leave
- **THEN** the request succeeds and they leave the network

#### Scenario: The entry points while the gate stands

- **WHEN** an account outside the group views their account or profile
- **THEN** neither the navigation entry nor the invitation is shown
- **AND** the catalogue listing carries `noindex`

## MODIFIED Requirements

### Requirement: The control is reachable without a direct URL

The Talent Network SHALL be reachable from the account's profile page: a candidate who is
not a member SHALL see an invitation to join there, and a member SHALL see their standing
and a link to their own card.

This is a requirement and not a nicety: a control reachable only by typing its path is
indistinguishable from a control that does not exist, and the catalogue's population
depends entirely on this. The profile page is the sole entry point — the account
navigation no longer carries a separate Talent Network section, since the invitation
already states standing and links to the control from the page where a candidate finishes
describing themselves.

#### Scenario: A non-member opens their profile

- **WHEN** a candidate who is not a member opens `/my/profile`
- **THEN** the page shows an invitation stating what joining publishes and what it
  withholds
- **AND** the invitation leads to the membership control

#### Scenario: A member opens their profile

- **WHEN** a member opens `/my/profile`
- **THEN** the page states that they are in the Talent Network
- **AND** offers to open their own public card as a visitor would see it

#### Scenario: A signed-in candidate opens their account

- **WHEN** any signed-in candidate views a page under `/my`
- **THEN** the account navigation lists no Talent Network section
