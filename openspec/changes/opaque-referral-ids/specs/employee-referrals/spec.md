## ADDED Requirements

### Requirement: A referral is addressed by an unguessable id

A referral offer and a referral request SHALL each be identified by a random id
rather than a sequential one. An id that is not well-formed SHALL be reported as
not found, so "not an id" and "not visible to you" remain one answer.

This matters more here than for a resource read only by its owner. An incoming
request's CV is deliberately served to **another person** — an approved referrer
of the company the request is addressed to — so its authorization is a
membership question rather than an ownership one. A countable id would turn a
single mistake in that check into bulk retrieval of other seekers' résumés
instead of one failed request.

#### Scenario: Two referrals get unrelated ids

- **WHEN** a seeker files two referral requests in a row
- **THEN** their ids are independently random, so neither reveals the other nor how many requests exist

#### Scenario: A malformed id is missing, not invalid

- **WHEN** a request names a referral id that is not well-formed — a number, or anything that is not an id
- **THEN** it is refused as not found, indistinguishable from one the caller may not see

#### Scenario: Who may read what is unchanged

- **WHEN** an approved referrer of the company opens an incoming request's CV, and a signed-in user who is not one attempts the same
- **THEN** the referrer receives the CV and the other is refused, exactly as before the ids changed
