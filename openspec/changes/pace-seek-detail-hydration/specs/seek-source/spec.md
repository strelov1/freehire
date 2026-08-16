## MODIFIED Requirements

### Requirement: Descriptions hydrated only for postings the catalogue lacks

The listing carries no description, only a one-line teaser, so the adapter SHALL implement
`HydratingSource` and fetch a posting's body from SEEK's GraphQL `jobDetails` operation ONLY when
the pipeline reports that posting as not yet ingested. A posting the catalogue already holds MUST be
returned marked `SeenRefresh`, so the pipeline refreshes its liveness without re-fetching or
overwriting the body hydrated when it was new.

The detail fetch MUST be rate-paced through a limiter shared by every board in a run, so the run's
aggregate request rate stays under SEEK's burst window independently of the detail pool's
concurrency.

A posting whose description could not be fetched MUST NOT be ingested. It is dropped for that run,
so the next crawl still reports it as new and retries it. This is the opposite of the rule the other
hydrating adapters follow, and it is deliberate: that rule trades a rare missing body for keeping the
posting, but SEEK's endpoint refuses in bursts, so ingesting body-less would strand whole slices with
no description permanently — the `seen` predicate reports only row existence, never whether the row
carries a body, so nothing downstream can repair it.

#### Scenario: New posting is hydrated

- **WHEN** the crawl yields a posting the catalogue does not hold
- **THEN** the adapter fetches its description and returns the job with that body

#### Scenario: Known posting costs no detail request

- **WHEN** the crawl yields a posting the catalogue already holds
- **THEN** the adapter returns it marked `SeenRefresh` and issues no detail request for it

#### Scenario: Failed detail defers the posting to the next crawl

- **WHEN** a posting's detail request fails or returns no content
- **THEN** the adapter omits that posting from the run, so it is not stored body-less and the next
  crawl retries it as new

#### Scenario: Detail requests are paced across the whole run

- **WHEN** many boards hydrate postings concurrently in one run
- **THEN** every detail request passes through one shared rate limiter, so the run's aggregate rate
  stays under SEEK's window rather than scaling with the number of boards in flight
