## MODIFIED Requirements

### Requirement: The jobs search endpoint translates facet params into one filter

The public job search endpoint (`GET /api/v1/jobs/search`) and the agent search
endpoint SHALL translate their facet query params into a single search-engine
filter through one pure builder shared with the saved-search digest matcher, so
the handler, a stored search, and a notification subscription built from the same
canonical query string produce identical filters. Within a facet, included values
SHALL OR into one group (or AND into per-value groups under `<param>_mode=and`);
facets SHALL AND across groups; the location facets (regions, countries, cities,
metros) SHALL share one ORed group. Excluded values SHALL each form their own AND
group.

The filter language SHALL additionally honour the modifier
`work_mode_remote_anywhere=true`. When it is set, `remote` is among the included
`work_mode` values, at least one physical mode (hybrid, onsite) is included, and
at least one location facet carries an include, the work-mode group SHALL become
the disjunction `work_mode IN [selected modes] OR (work_mode IN [onsite,
hybrid] AND <location group>)` — remote postings from anywhere, plus physical
postings within the chosen geography. The modifier SHALL be a no-op when any of
those preconditions fails, and SHALL be ignored when `work_mode_mode=and` is set.
Location excludes SHALL keep their ordinary AND meaning (excluding a geography
applies to every arm of the disjunction).

#### Scenario: Remote anywhere with local physical work

- **WHEN** a request carries `work_mode=remote,hybrid&countries=de&work_mode_remote_anywhere=true`
- **THEN** the filter is `(work_mode IN ["remote", "hybrid"] OR (work_mode IN ["hybrid", "onsite"] AND countries = "de"))`-shaped: a remote posting in Brazil matches, a hybrid posting in Germany matches, an on-site posting in Brazil does not, and a hybrid posting in France does not

#### Scenario: No location include leaves the modifier inert

- **WHEN** a request carries `work_mode=remote,hybrid&work_mode_remote_anywhere=true` with no location facet
- **THEN** the work-mode group is the plain OR over the selected modes, identical to the modifier's absence

#### Scenario: Remote not among the modes leaves the modifier inert

- **WHEN** a request carries `work_mode=hybrid,onsite&countries=de&work_mode_remote_anywhere=true`
- **THEN** the work-mode group is the plain OR over hybrid and onsite, as if the modifier were absent

#### Scenario: AND mode wins over the modifier

- **WHEN** a request carries `work_mode=remote,hybrid&work_mode_mode=and&countries=de&work_mode_remote_anywhere=true`
- **THEN** the work mode is two AND groups (a job must be both remote and hybrid — effectively none), and the modifier adds nothing

#### Scenario: Location excludes apply to every arm

- **WHEN** a request carries `work_mode=remote,hybrid&countries=de&countries_exclude=us&work_mode_remote_anywhere=true`
- **THEN** remote postings in the US are excluded along with the physical ones — the exclude is its own AND group beside the disjunction

#### Scenario: The digest matcher applies the same disjunction

- **WHEN** a saved search's stored query carries the modifier and the notification matcher replays it
- **THEN** the matcher's filter equals the handler's filter for the same query string, because both come from the one pure builder

#### Scenario: The modifier alone narrows nothing

- **WHEN** a saved search's stored query is `work_mode_remote_anywhere=true` with no other filter
- **THEN** the query reads as non-narrowing (`Narrows` false), the same rule every other modifier follows

#### Scenario: An unknown modifier value is reported, not applied

- **WHEN** a request carries `work_mode_remote_anywhere=yes`
- **THEN** the modifier is not applied, and the value is reported in `meta.ignored_values` — the same value-level report every closed-vocabulary facet follows
