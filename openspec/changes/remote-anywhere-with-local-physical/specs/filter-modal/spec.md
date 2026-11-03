## MODIFIED Requirements

### Requirement: The job filters are edited in a two-pane modal grouped into sections

(The Work format section of the modal gains one control.)

The modal's Work format pane SHALL offer a "Remote from anywhere" checkbox whenever
Remote and at least one physical format (Hybrid, On-site) are both selected AND at
least one location facet (Region, Country, City, Metro) carries a selected value —
the condition under which the modifier changes the answer. The checkbox SHALL
serialize to `work_mode_remote_anywhere=true`, survive the URL round trip, count
as one active filter when set, and show a summary chip so a filter set that
arrived from a shared link or a saved search reads its effect. The checkbox SHALL
NOT appear outside the condition: a toggle that changes nothing reads as broken.

#### Scenario: The checkbox appears for remote plus physical plus a location

- **WHEN** the user selects Remote and Hybrid and picks a country in the modal
- **THEN** the Work format pane shows the "Remote from anywhere" checkbox

#### Scenario: The checkbox is hidden when it would change nothing

- **WHEN** the user selects only Remote, or no location facet is selected
- **THEN** the Work format pane shows no "Remote from anywhere" checkbox

#### Scenario: The chip summarizes an inherited modifier

- **WHEN** a shared link carrying `work_mode=remote,hybrid&countries=de&work_mode_remote_anywhere=true` is opened
- **THEN** the filter summary shows a "Remote from anywhere" chip whose removal unsets the modifier

### Requirement: Apply my profile flattens the location block without losing the remote reach

"Apply my profile" SHALL flatten the profile's location block so that a user who
accepts remote AND at least one physical format AND states a base or relocation
geography gets remote postings within their remote reach (anywhere, when none is
stated) plus physical postings within the base and relocation geography — by
staging the `work_mode_remote_anywhere` modifier, the reach as
`work_mode_remote_regions`/`work_mode_remote_countries`, and only the base and
relocation geography as the location facets. A profile without that shape (remote
only; physical only; no physical geography) SHALL flatten exactly as before. When
it stages the modifier it SHALL NOT stage the relocation facet: that facet ANDs
around the whole disjunction and would demand relocation from remote postings; the
relocation targets remain in the physical arm's geography.

#### Scenario: Hybrid at home plus remote anywhere from the profile

- **WHEN** a user whose profile lists work_modes remote+hybrid, a base country DE, and no remote reach applies their profile
- **THEN** the staged filters carry work_mode remote+hybrid, countries DE, and `work_mode_remote_anywhere=true` with no remote reach

#### Scenario: A stated remote reach bounds only the remote arm

- **WHEN** a user whose profile lists work_modes remote+hybrid, a base country DE, and remote reach Europe applies their profile
- **THEN** the staged filters carry the modifier, `work_mode_remote_regions=eu` and countries DE — remote work within Europe, hybrid only in Germany

#### Scenario: The chip names the reach

- **WHEN** the modifier is in force with `work_mode_remote_regions=eu`
- **THEN** the summary chip and the modal checkbox read "Remote within Europe", and removing the chip clears the modifier and the reach together

#### Scenario: Remote-only profile stays global

- **WHEN** a user whose profile lists work_modes remote only with a base country applies their profile
- **THEN** the staged filters carry work_mode remote with no base-country narrowing and no modifier — unchanged from today's behaviour
