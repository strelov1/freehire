## ADDED Requirements

### Requirement: Sharing is an opt-in the member can withdraw

A member's applications SHALL contribute to the process signal only while the account carries a
consent timestamp, which is NULL by default. Consent SHALL be given and withdrawn only through a
cookie-authenticated request; an API key of any scope, and the in-app assistant, SHALL NOT be able
to set it. A member SHALL be able to exclude any single application from the signal. Withdrawal
and exclusion SHALL take effect at the next rollup, and the product SHALL say "within a day".

The aggregate SHALL NOT store any per-member row, so withdrawal, exclusion and account deletion
need no separate erasure step.

#### Scenario: A new account contributes nothing

- **WHEN** a member has never given consent
- **THEN** none of their applications is counted in any figure

#### Scenario: An API key cannot enrol a member

- **WHEN** a request authenticated by an API key asks to give consent
- **THEN** it is refused and the consent timestamp is unchanged

#### Scenario: Withdrawal removes the member at the next rollup

- **WHEN** a member withdraws consent and the rollup next runs
- **THEN** their applications are in no cluster's figure

#### Scenario: One application is kept out

- **WHEN** a consenting member excludes one application
- **THEN** that application is counted in neither the numerator nor the denominator of any figure
- **AND** their other applications still count

### Requirement: The unit is the role cluster, over a rolling window

The signal SHALL be computed per role cluster: the ultimate owner of a posting's duplicate closure,
resolved by the same membership `GET /jobs/:slug/copies` lists, not by `role_fingerprint`. Only
applications dated within the last 180 days SHALL count. A request for a suppressed posting SHALL
be answered with its owner's cluster.

Each member SHALL count at most once per cluster, by their earliest application in it, however
many copies of the role they applied to.

#### Scenario: Copies of one role pool

- **WHEN** members applied to the Berlin and the Munich copy of one clustered role
- **THEN** both applications contribute to one cluster's figure

#### Scenario: One member applying to two copies is one contributor

- **WHEN** one member applied to two copies in the same cluster
- **THEN** they count once toward the contributor floor and once in each share

#### Scenario: An old application ages out

- **WHEN** an application was made more than 180 days ago
- **THEN** it is in no figure

### Requirement: Only observable, mature, evidence-sourced applications count

An application SHALL count only when its member has a connected Gmail or hosted mailbox, the
application is older than the `applied` silence threshold of `internal/job/silence`, and its posting is not
private. A reply SHALL count as an employer reply only when its event source is `mail_gmail` or
`mail_hosted`; a calendar interview SHALL count as an interview. A stage typed by the member, and
any `mail_external` event, SHALL NOT affect any figure. Applications that do not count SHALL be
excluded from both sides of every ratio.

#### Scenario: A recent application is not read as silence

- **WHEN** an application is younger than the silence threshold and has no reply
- **THEN** it is in neither the numerator nor the denominator

#### Scenario: A pushed reply is ignored

- **WHEN** an `employer_reply` event has source `mail_external`
- **THEN** its application is not counted as answered, and a figure computed with and without
  that event is identical

#### Scenario: A typed stage is ignored

- **WHEN** a member sets an application's stage to `rejected` by hand and no mail or calendar
  event records it
- **THEN** the application counts as `no_reply`

### Requirement: Nothing is published below the floor, and what is published is quantized

A cluster's figure SHALL exist only with at least 10 distinct contributors. Below that the cluster
SHALL have no stored row, so an absent figure means only "not enough to say". The reply-time range
SHALL additionally require at least 5 answered contributors, and is otherwise omitted while the rest
of the figure stands.

A published figure SHALL consist of: a contributor band (`10-24`, `25-49`, `50+`); the share that
heard back and the shares `no_reply`, `rejected` and `advanced`, each rounded to the nearest 10%
and summing to 100; a reply-time range of the 25th and 75th percentile in whole days; and the
window length. It SHALL NOT contain an exact count, a per-stage or per-day count, or any separate
figure for offers or acceptances. `advanced` SHALL fold screening, assessment, interview and offer.

#### Scenario: Nine contributors publish nothing

- **WHEN** a cluster has nine distinct contributors
- **THEN** no row is stored and the endpoint answers `{"data": null}`

#### Scenario: Reply time needs its own floor

- **WHEN** a cluster has twelve contributors of whom four were answered
- **THEN** the figure is published without `reply_days`

#### Scenario: Offers are not separated

- **WHEN** a cluster's contributors include members who received an offer
- **THEN** the offers appear only inside `advanced`, and no field distinguishes them

#### Scenario: The response carries no raw count

- **WHEN** the endpoint answers for a cluster above the floor
- **THEN** the body contains no integer field other than bands, rounded percentages, whole days
  and the window length

### Requirement: The figure is served from a daily rollup, as its own resource

The signal SHALL be recomputed in full at least daily by an atomic delete-and-reinsert and SHALL be
served by `GET /jobs/:slug/process-signal`, public and independent of the job wire shape. The
endpoint SHALL return the stored figure and nothing computed at request time. It SHALL return
`{"data": null}` for a private posting, an on-site employer vacancy, a cluster a moderator has
suppressed, and any cluster without a row.

#### Scenario: The job payload is unchanged

- **WHEN** a client requests `GET /jobs/:slug` or a search result
- **THEN** the response carries no process-signal field

#### Scenario: An on-site employer vacancy has no figure

- **WHEN** the posting is an employer vacancy taking applications on freehire
- **THEN** the endpoint answers `{"data": null}` whatever the ledger holds
