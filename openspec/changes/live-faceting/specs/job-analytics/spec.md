## ADDED Requirements

### Requirement: Disjunctive facet distribution

The facet-distribution endpoint SHALL support a **disjunctive** mode (opt-in via a
`disjunctive` query flag). In this mode, each requested facet's distribution SHALL
be computed under the full filter **with that facet's own selection removed**
(its `<param>`, `<param>_exclude`, and `<param>_mode` values excluded), so a
facet's own selection does not zero out its sibling values. The reported `total`
SHALL still be the estimated count under the **full** filter (all facets applied)
— the number the "Show N results" action reflects. Non-disjunctive requests keep
the existing conjunctive behaviour (every facet counted under the full filter).

The disjunctive distributions SHALL be produced by running the per-facet queries
**concurrently** (a `search` capability), so the endpoint's latency is that of a
single facet query rather than the sum of all of them.

#### Scenario: A facet's own selection does not hide its siblings

- **WHEN** a client requests the disjunctive facet distribution with
  `seniority=senior` selected
- **THEN** the `seniority` distribution still reports counts for the other
  seniorities (e.g. `junior`, `middle`) — each counted under the rest of the
  filter, ignoring the `senior` selection
- **AND** a different facet (e.g. `category`) is counted under a filter that
  **does** include `seniority=senior`

#### Scenario: The total reflects the full filter

- **WHEN** a client requests the disjunctive distribution with several facets
  selected
- **THEN** the response `total` is the estimated job count under all selected
  facets combined (not any single facet's disjunctive subtotal)

#### Scenario: Non-disjunctive requests are unchanged

- **WHEN** a client requests the facet distribution without the disjunctive flag
- **THEN** every facet is counted under the full filter (conjunctive), exactly as
  before
