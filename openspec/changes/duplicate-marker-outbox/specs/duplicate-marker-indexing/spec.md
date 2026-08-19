## ADDED Requirements

### Requirement: A change in duplicate status reaches the index without a rebuild

The system SHALL queue a posting for the facet index the moment its duplicate status changes,
rather than waiting for the next full rebuild. A posting that becomes a non-canonical
duplicate SHALL be queued for removal; a posting that becomes canonical again SHALL be queued
for indexing. Queueing SHALL happen in the same statement that writes the marker, so a run
that fails partway cannot leave a status change unqueued.

#### Scenario: A posting that becomes a duplicate leaves search

- **WHEN** a dedup pass marks an open, canonical posting as a duplicate
- **THEN** that posting is queued for removal from the facet index, and the drain removes its
  document on its next run rather than the next rebuild

#### Scenario: A posting that becomes canonical returns to search

- **WHEN** a dedup pass clears the last marker on a posting — for example its canon closed
  and it is now the canon itself
- **THEN** that posting is queued for indexing, and the drain restores its document

#### Scenario: Re-pointing a duplicate queues nothing

- **WHEN** a pass changes which canon a posting points at, and the posting is a duplicate both
  before and after
- **THEN** nothing is queued: the document is already absent from the index, so removing it
  again is work with no effect

#### Scenario: An unchanged pass run queues nothing

- **WHEN** a marker refresh runs over a catalogue whose duplicate statuses have not changed
- **THEN** no rows are queued on either queue

### Requirement: The queueing decision reads the derived marker

The queueing decision SHALL be made from the derived `jobs.duplicate_of` before and after the
write, not from the owning column the pass itself writes. A pass clearing its own column does
not make a posting canonical while another pass still holds a marker on it, and treating that
as a return to canonical would put a posting that is still a duplicate back into search.

#### Scenario: One pass releases while another still holds

- **WHEN** the aggregator pass releases a posting it had suppressed, but the role pass still
  marks that posting as a repost
- **THEN** the posting is NOT queued for indexing, because it is still a duplicate

#### Scenario: The last marker released returns the posting

- **WHEN** the aggregator pass releases a posting and no other pass holds a marker on it
- **THEN** the posting is queued for indexing

### Requirement: Duplicates already in the index remain the rebuild's job

This change SHALL NOT backfill postings whose duplicate status predates it. A posting already
marked and already indexed has no transition to observe, and the scheduled full rebuild
continues to collapse it exactly as before.

#### Scenario: A pre-existing duplicate is not queued

- **WHEN** a marker refresh runs over a posting that was already a duplicate before this
  change shipped, and its status does not change
- **THEN** nothing is queued for it, and its document is removed by the next scheduled
  rebuild as it would have been anyway
