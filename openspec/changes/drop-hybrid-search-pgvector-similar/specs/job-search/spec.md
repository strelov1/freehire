## REMOVED Requirements

### Requirement: Hybrid keyword and semantic search

**Reason**: The `jobs_semantic` Meilisearch index this requirement depended on is
removed entirely (see the `semantic-embedding` and `similar-jobs` capability
changes in this same OpenSpec change). General keyword+facet search over the
`jobs` index was already effectively keyword-only in production (the SPA never
sent a non-zero `semantic_ratio`); this formalizes that as the permanent
contract rather than an optional blend that happened to default off.

**Migration**: No user-facing migration — behavior is unchanged for every
existing caller, since `semantic_ratio` was already defaulting to `0`
everywhere it mattered. The `semantic_ratio` query parameter is removed from
`GET /api/v1/jobs/search`; the handler simply stops reading it, so a client
that still sends it has the value silently unused rather than rejected — no
different from any other unrecognized query parameter today.
