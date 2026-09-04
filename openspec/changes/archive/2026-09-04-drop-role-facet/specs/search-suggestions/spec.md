## REMOVED Requirements

### Requirement: A category is not offered when a role already names it

**Reason**: There are no role suggestions to collide with. The rule was right while
both kinds existed — measured on the live catalogue, a bare-category role and its
category select the same postings to the digit — but it resolved the collision in
favour of the kind that is now gone, and the dictionary it produced held **zero**
categories as a result.

**Migration**: Specializations become offerable suggestions for the first time. Nothing
replaces the rule; the collision it arbitrated no longer exists.

### Requirement: One suggestion per base role, never one per grade

**Reason**: There are no graded role slugs to collapse.

**Migration**: The grade axis is the `seniority` facet, and a graded phrase reaches the
dictionary the way every other phrase does — as a mined posting title. "Senior Software
Engineer" is 23,643 postings written that way, which is a stronger answer than a slug
built by multiplying two vocabularies.

### Requirement: A dedicated suggestions index holds every offerable completion

**Reason**: Its `kind` vocabulary named `role`, and one of its two scenarios — "A role
suggestion carries its slug" — asserts a document kind the builder can no longer
produce. Rewording that scenario inside a MODIFIED block would have dropped it silently
rather than recording that what it described is gone, so the requirement is removed and
restated with the surviving kinds.

**Migration**: The index, its separateness argument and every other kind are unchanged.
Only `role` leaves the vocabulary.

## ADDED Requirements

### Requirement: A dedicated suggestions index holds every offerable completion kind

The system SHALL maintain a Meilisearch index, separate from the jobs index, holding
one document per offerable suggestion. Each document SHALL carry the display text, a
`kind` of `title`, `skill`, `category` or `company`, the facet value the suggestion
applies (absent for a `title`, which is free text), the count of open postings behind
it, and the count of times visitors have searched for it.

The index SHALL be separate rather than a facet on the jobs index. A facet is a bounded
value dictionary and distinct job titles number in the millions: `MaxValuesPerFacet`
would truncate the distribution and `title` is not a filterable attribute. Suggestions
are mined into a bounded dictionary offline instead.

#### Scenario: A title suggestion carries no facet value
- **WHEN** the index holds the suggestion "Java Developer" of kind `title`
- **THEN** it carries no facet value, because no facet names that phrase

#### Scenario: A category suggestion carries its slug
- **WHEN** the index holds the suggestion "Backend" of kind `category`
- **THEN** it carries the facet value `backend`

#### Scenario: No suggestion names a role
- **WHEN** the dictionary is built
- **THEN** no document carries the kind `role`
