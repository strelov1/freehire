## ADDED Requirements

### Requirement: Recognize a jobs.dover.com board link

The system SHALL recognize a link on `jobs.dover.com` shaped `/<Org>/<uuid>` — with an
optional trailing posting slug — as a `dover` board whose id is the first segment alone (the
company's Dover slug, kept as the link spells it), the same board id an `app.dover.com` link
for the same company spells, and whose canonical URL collapses to the `/<Org>` board address,
so a posting link and the board listing map to one board. The UUID-shaped second segment SHALL
be required: a bare `/<Org>` is a marketing shell the platform serves for any slug, and the
vendor's own pages (`/startup-jobs`) name no tenant. The uuid itself SHALL NOT be folded into
the board id: it may be the client id or a posting id, and the slug is the identity both
Dover hosts share.

#### Scenario: The submitted Deepspatial board link is recognized

- **WHEN** a user submits `https://jobs.dover.com/Deepspatial/a5546acf-45c3-44ff-bedd-a4314fd0f37d`
- **THEN** it derives source `dover`, board `Deepspatial`, canonical
  `https://jobs.dover.com/Deepspatial`

#### Scenario: A posting link maps to the same board

- **WHEN** a user submits a `jobs.dover.com` link with a trailing posting slug after the uuid
- **THEN** it derives the same source, board, and canonical as the bare two-segment link

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

#### Scenario: Both Dover hosts file under one board

- **WHEN** a user submits `https://jobs.dover.com/Deepspatial/a5546acf-…` and another submits
  `https://app.dover.com/jobs/Deepspatial`
- **THEN** both derive source `dover`, board `Deepspatial`, so the second is a duplicate of
  the first
