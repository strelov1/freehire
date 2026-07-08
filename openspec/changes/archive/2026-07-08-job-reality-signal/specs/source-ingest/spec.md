## ADDED Requirements

### Requirement: The write path persists a repost-clustering role fingerprint
The `UpsertJob` write path SHALL compute and persist a narrow `role_fingerprint` for every job — derived from company, normalized title, and description, and deliberately excluding volatile fields (`posted_at`, url, public slug) — so reposts of the same role under new `external_id`s cluster to one fingerprint.

#### Scenario: The fingerprint ignores a bumped posted date
- **WHEN** a role is reposted under a new `external_id` with an identical title and description but a refreshed `posted_at`
- **THEN** both postings resolve to the same `role_fingerprint`

#### Scenario: content_hash is not reused as the fingerprint
- **WHEN** the write path computes the fingerprint
- **THEN** it uses a dedicated narrow fingerprint (not `content_hash`, which includes `posted_at` and so changes on repost)

#### Scenario: A distinct role gets a distinct fingerprint
- **WHEN** two postings differ in normalized title or description
- **THEN** they resolve to different `role_fingerprint` values
