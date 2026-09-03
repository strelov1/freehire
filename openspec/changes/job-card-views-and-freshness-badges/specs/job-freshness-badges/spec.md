## ADDED Requirements

### Requirement: The freshness badge pair and its thresholds

The system SHALL offer two freshness badges on a posting: `New` and `Be an early
applicant`. Both are derived at read time from fields the posting already
carries, and neither is stored, indexed, or served as a field of its own.

`New` SHALL be shown when the posting is at most 7 days old. A posting is not
"new" for longer than that on a board where the median role stays open for a
month — beyond a week the word stops carrying information.

`Be an early applicant` SHALL be shown when the posting is at most 3 days old AND
at most 3 users have marked the job applied. It SHALL only ever accompany `New`,
never appear alone, because its window is strictly inside `New`'s.

The age SHALL be measured in whole days from the served `posted_at`, which is
already the effective posting date (the source's own date, or the ingest date when
the source gives none or gives one in the future). The measured age SHALL be
clamped at zero: a source clock running a few hours ahead is ordinary and MUST NOT
read as a posting from the future. A posting whose `posted_at` is missing or
unparseable SHALL receive neither badge — an unknown age is not a fresh one.

Each badge SHALL carry a tooltip stating the fact behind it rather than
restating the label.

This requirement records behaviour that already exists in the codebase and had no
specification. The thresholds are unchanged by the change that introduces this
spec.

#### Scenario: A posting from yesterday carries both badges

- **WHEN** the badges are computed for a job posted 1 day ago with 0 applied
  marks and a fresh reality signal
- **THEN** both `New` and `Be an early applicant` are returned

#### Scenario: A five-day-old posting is new but not early

- **WHEN** the badges are computed for a job posted 5 days ago with a fresh
  reality signal
- **THEN** only `New` is returned

#### Scenario: A posting past the new window carries neither badge

- **WHEN** the badges are computed for a job posted 20 days ago
- **THEN** no badge is returned

#### Scenario: A well-applied fresh posting is new but not early

- **WHEN** the badges are computed for a job posted 1 day ago that 9 users have
  marked applied, with a fresh reality signal
- **THEN** only `New` is returned

#### Scenario: A future-dated posting reads as posted today

- **WHEN** the badges are computed for a job whose `posted_at` is a few hours in
  the future
- **THEN** its age is treated as zero days and `New` is returned

#### Scenario: An unknown posting date yields no badge

- **WHEN** the badges are computed for a job whose `posted_at` is absent or
  unparseable
- **THEN** no badge is returned

### Requirement: The reality signal gates both badges

When a posting carries the job-reality signal, a signal reporting anything other
than `fresh` — or reporting `fake_freshness` — SHALL suppress both badges
regardless of the posting date.

The reality signal is the system's own reading of how long a job has actually
been open, and it recognises the case the posting date alone cannot: a role open
for eight months whose source rewrites its posting date on every crawl. Trusting
`posted_at` there would print `New` on the oldest job in the catalogue. Both
badges read as encouragement, so both are held to a higher bar than "the number
looks small".

#### Scenario: A stale posting with a fresh date gets no badge

- **WHEN** the badges are computed for a job posted 1 day ago whose reality
  signal does not report `fresh`
- **THEN** no badge is returned

#### Scenario: A fake-freshness posting gets no badge

- **WHEN** the badges are computed for a job posted 1 day ago whose reality
  signal reports `fresh` with `fake_freshness` set
- **THEN** no badge is returned

### Requirement: "Be an early applicant" claims only what freehire can observe

The applied count behind `Be an early applicant` is the number of signed-in users
who marked the job applied **on freehire**. It cannot see the employer's inbox, so
it is a floor on the real number and never a measure of it.

The badge SHALL therefore be offered as an invitation rather than a statement of
fact, and its tooltip SHALL name who was counted, so the imprecise headline is one
hover away from the precise basis. The wording SHALL NOT imply knowledge of the
employer's applicant pool.

The threshold SHALL be small enough that the claim survives being wrong by an
order of magnitude. This is why the badge is bounded by a count of 3 rather than
a larger figure that merely "looks small".

#### Scenario: The tooltip names freehire as the source of the count

- **WHEN** `Be an early applicant` is computed for a job that 2 users have marked
  applied
- **THEN** its tooltip states that 2 people have told us they applied to this job

#### Scenario: The tooltip is explicit when nobody has applied

- **WHEN** `Be an early applicant` is computed for a job with 0 applied marks
- **THEN** its tooltip states that nobody has told us they applied yet

#### Scenario: The tooltip carries the posting age alongside the count

- **WHEN** `Be an early applicant` is computed for a job posted 2 days ago
- **THEN** its tooltip includes how long ago the job was posted

### Requirement: The badges render on the job card and the job detail page

Both the job card and the job detail page SHALL render the badge pair, computed
by one shared rule. Neither surface SHALL restate a threshold or the reality gate.
A second copy of a threshold is a second answer, and the two would drift — the
card and the detail page must never tell different stories about the same posting.

On the card the badges SHALL render inside the existing signal row under the
title, in this order: the reality or ghost chip first, then `New`, then `Be an
early applicant`, then the row's existing facet chips, employer credentials and
country flags. The reality/ghost chip leads because a warning that a posting may
not be real outranks a note that it is fresh; the badges precede the facet chips
because they are time-sensitive signals rather than stable attributes of the role.

The badges SHALL be visually distinct from the outline facet chips beside them,
rendered in the design system's brand-tinted badge variant.

The signal row's render guard SHALL account for the badges, so a job whose only
signal is a badge still opens the row. Without this a fresh posting carrying no
reality chip, no facets, no countries and no credentials would compute a badge and
then have nowhere to draw it.

#### Scenario: A ghost posting shows its warning first

- **WHEN** a card is rendered for a badge-earning job that also carries a ghost
  signal
- **THEN** the ghost chip precedes the `New` badge in the signal row

#### Scenario: A badge alone opens the signal row

- **WHEN** a card is rendered for a badge-earning job with no facet chips, no
  countries and no credentials
- **THEN** the signal row is rendered and contains the badges

#### Scenario: Badges are distinguishable from facet chips

- **WHEN** a card renders both badges and facet chips in the signal row
- **THEN** the badges use the brand-tinted variant and the facet chips the
  outline variant

### Requirement: A card without the reality signal shows no badges

On the job card, a posting that carries no reality signal at all SHALL receive no
badges, even when its date would earn them. The card SHALL require the signal
rather than letting the date stand alone.

This is stricter than the shared rule's own behaviour, which permits the date to
stand alone when the signal is absent. That permission is correct on the job
detail page, where the signal is always computed, and wrong on a card: the browse
feed is served from the search endpoint, whose hits carry the signal, but the
Postgres-backed jobs list and the tracking/assistant card projection do not.
Trusting the date on those surfaces would print `New` on precisely the postings
the reality gate exists to catch, and it would do so on the surface where jobs
are scanned fastest and least critically.

The consequence is deliberate: surfaces fed by a projection without the signal
show no badges rather than unverified ones. The requirement SHALL be expressed as
a testable function rather than a condition inside a template.

#### Scenario: A search-fed card earns its badges

- **WHEN** a card is rendered from a search hit carrying a fresh reality signal
  for a job posted 1 day ago
- **THEN** both badges are shown

#### Scenario: A card from a projection with no reality signal shows none

- **WHEN** a card is rendered from a listing projection that carries no reality
  signal, for a job posted 1 day ago
- **THEN** no badge is shown

#### Scenario: The detail page still lets the date stand alone

- **WHEN** the job detail page computes badges for a job with no reality signal
  posted 1 day ago
- **THEN** the shared rule's existing behaviour is unchanged and `New` is
  returned
