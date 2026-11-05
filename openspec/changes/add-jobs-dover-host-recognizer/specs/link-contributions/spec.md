## ADDED Requirements

### Requirement: Recognize a jobs.dover.com board link

The system SHALL recognize a link on `jobs.dover.com` shaped `/<Org>/<client-uuid>` — with an
optional trailing posting slug — as a `dover` board whose id is the two-segment pair
`<Org>/<client-uuid>` kept as the link spells it, and whose canonical URL collapses to the
two-segment board address, so a posting link and the board listing map to one board. A
`jobs.dover.com` URL that does not carry a UUID-shaped second segment SHALL NOT be recognized:
a bare `/<Org>` is a marketing shell the platform serves for any slug, and the vendor's own
pages (`/startup-jobs`) name no tenant.

#### Scenario: The submitted Deepspatial board link is recognized

- **WHEN** a user submits `https://jobs.dover.com/Deepspatial/a5546acf-45c3-44ff-bedd-a4314fd0f37d`
- **THEN** it derives source `dover`, board `Deepspatial/a5546acf-45c3-44ff-bedd-a4314fd0f37d`,
  canonical `https://jobs.dover.com/Deepspatial/a5546acf-45c3-44ff-bedd-a4314fd0f37d`

#### Scenario: A posting link maps to the same board

- **WHEN** a user submits a `jobs.dover.com` link with a trailing posting slug after the pair
- **THEN** it derives the same source, board, and canonical as the bare pair link

#### Scenario: A bare org segment is declined

- **WHEN** a user submits `https://jobs.dover.com/Deepspatial`
- **THEN** nothing is derived — the segment is a marketing shell served for any slug — and the
  link falls to manual review

#### Scenario: A non-UUID second segment is declined

- **WHEN** a user submits `https://jobs.dover.com/Deepspatial/not-a-uuid`
- **THEN** nothing is derived and the link falls to manual review

#### Scenario: The vendor's own listing is declined

- **WHEN** a user submits `https://jobs.dover.com/startup-jobs`
- **THEN** nothing is derived and the link falls to manual review
