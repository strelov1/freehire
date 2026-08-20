## MODIFIED Requirements

### Requirement: The rail carries an Experience pane

The filter modal's `ROLE` section SHALL carry an `Experience` rail entry whose pane
renders three labelled controls, in order: the `seniority` pills, the `role_type`
pills, and a years-of-experience control bounding how much experience a posting asks
for. The pane SHALL be reachable by the same rail interaction as every other entry,
and its rail entry's staged count SHALL cover all three controls together.

The role-type pills sit directly beneath the seniority pills because the two are the
axes users most often conflate: "Lead" reads to many as a management grade, while in
this catalogue it names the individual-contributor ladder. Rendering them adjacent
makes the two questions visibly separate.

Moving the seniority pills into this pane SHALL NOT change the `seniority` URL
parameter, its allowed values, or its exclusion (`seniority_exclude`) behaviour — a
URL that selects a seniority before this change SHALL select the same seniority
after it.

#### Scenario: The Experience pane renders all three controls

- **WHEN** the user selects the `Experience` rail entry
- **THEN** the right pane renders the seniority pills, the role-type pills, and the
  years-of-experience control, in that order

#### Scenario: An existing seniority URL still applies

- **WHEN** a filter URL carrying `seniority=senior` is loaded after the move
- **THEN** the senior pill is selected in the `Experience` pane and the job list is
  filtered to senior postings, exactly as before the move

#### Scenario: The rail count sums the pane's controls

- **WHEN** the user stages one seniority value, the role-type value, and a years
  bound
- **THEN** the `Experience` rail entry shows the count `3`

## ADDED Requirements

### Requirement: Role type is a single three-state pill

The role-type control SHALL render the one value of `vocab.RoleTypeValues` as a
single pill, using the same three-state chip every other facet uses: off, included,
excluded. A second pill for individual contributors SHALL NOT be added.

The excluded state means "no people-management marker in the title". The control
SHALL NOT label it as individual-contributor work, in the pill, the summary chip, or
any accompanying text: the catalogue can show that a posting IS a management role
and cannot show that it is not.

#### Scenario: The pill cycles through three states

- **WHEN** the user clicks the role-type pill once, twice, then a third time
- **THEN** the value is included, then excluded, then cleared — matching every other
  facet pill

#### Scenario: Excluding is not presented as individual contributor

- **WHEN** the role-type value is excluded and the summary chip is rendered
- **THEN** neither the chip nor the pane describes the result as
  individual-contributor postings
