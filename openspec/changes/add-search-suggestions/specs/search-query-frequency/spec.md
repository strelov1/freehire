## ADDED Requirements

### Requirement: What visitors search for is recorded durably

The system SHALL record each free-text search query in a `search_queries` table:
the normalised query text as the key, a count of how often it has been seen, and
when it was last seen. The write SHALL be an upsert on every request carrying a
non-empty `q`.

The normalisation SHALL be the same function the suggestions builder applies to
mined titles, so a typed query and a mined title land on the same key and their
counts describe the same phrase.

Measured on five days of production access logs, the site served 71,174 searches
across 8,340 distinct queries — the table is small by construction.

#### Scenario: A repeated query increments rather than duplicates
- **WHEN** the same normalised query is searched twice
- **THEN** the table holds one row for it with a count of two

#### Scenario: A query and a mined title share a key
- **WHEN** a visitor searches `Java Developer` and the builder mines the title `java developer`
- **THEN** both resolve to the same normalised key

### Requirement: The frequency record identifies no one

The table SHALL store the query text, its count and its last-seen timestamp, and
nothing else. It SHALL NOT store a user id, a session id, or an IP address: the
record exists to say what the catalogue is asked for, not who asked.

#### Scenario: No identifier is written
- **WHEN** a signed-in user runs a search
- **THEN** the recorded row carries no reference to that user

### Requirement: Recording a search never fails the search

Recording SHALL NOT be able to fail or delay the search response. A write error
SHALL be logged and discarded; the search result is what the visitor asked for and
the measurement is a by-product.

#### Scenario: A failed write still returns results
- **WHEN** the frequency write fails
- **THEN** the search response is served normally

### Requirement: The builder reads frequency into the suggestions index

The suggestions builder SHALL join each suggestion to its recorded search count
and write that count into the index document, so the endpoint can rank by demand
without querying the database per request.

A suggestion nobody has searched for SHALL carry a zero rather than be excluded —
absence of demand is not evidence against a suggestion, and the open-posting count
still orders it.

#### Scenario: An unsearched suggestion is still offered
- **WHEN** a suggestion has no row in `search_queries`
- **THEN** it is written to the index with a search count of zero and remains offerable
