## ADDED Requirements

### Requirement: The aggregate's write mapping owns every derived column

The mapping from a `Job`'s readable projection to its persistence parameters SHALL
compute every derived column a stored posting carries — the content fingerprint
(`content_hash`) and the role-identity fingerprint (`role_fingerprint`) — rather than
documenting that each caller must set them after the mapping returns. A write path
SHALL NOT be able to persist a posting missing either column by omitting a step.

This SHALL hold for every write path without exception: automated ingest, Telegram
extraction, URL import, moderator authoring, and moderator editing. The mapping SHALL
be the only producer of these two columns on the write path, so the same posting
content yields the same fingerprints regardless of which path wrote it.

Because the content fingerprint covers the source posted time, a caller that supplies
a posted time outside the deterministic derivation SHALL supply it through the
aggregate's draft, before the mapping runs — not by overwriting the mapped parameters
afterwards, which would fingerprint a posted time other than the one written.

#### Scenario: A moderator-authored vacancy carries both derived columns

- **WHEN** a moderator creates a hand-curated vacancy
- **THEN** the stored row carries a non-empty `role_fingerprint` and a non-empty
  `content_hash`, so the vacancy clusters against the automated copy of the same role
  and is eligible for the content-change re-embed

#### Scenario: A moderator edit refreshes the content fingerprint

- **WHEN** a moderator edits the description of a hand-curated vacancy that has
  already been embedded
- **THEN** the stored `content_hash` changes to match the edited content, so the
  semantic index re-embeds the vacancy instead of permanently describing the pre-edit
  text

#### Scenario: A caller-supplied posted time is inside the fingerprint

- **WHEN** a write path that takes the posted time from its source (Telegram
  extraction, URL import) persists a posting
- **THEN** the stored `content_hash` is the fingerprint of the posted time that was
  written, and re-ingesting the same posting unchanged reports no content change

#### Scenario: Identical content fingerprints identically across write paths

- **WHEN** two write paths persist postings with identical mapped content
- **THEN** both rows carry the same `content_hash` and the same `role_fingerprint`,
  because a single mapping computed them
