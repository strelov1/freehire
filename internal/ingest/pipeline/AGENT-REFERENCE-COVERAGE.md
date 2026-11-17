# Pipeline conventions: aggregator postings

Part of [AGENT-REFERENCE.md](AGENT-REFERENCE.md#always-true).

An employer having another open vacancy never proves that an aggregator posting is a duplicate.
The pipeline saves readable, catalogue-eligible postings and refreshes re-listed identities,
regardless of other sources covering the employer. It must not close them as `ats_covered`.

Posting-level duplicate detection remains in the store and the periodic marker passes:
identity, role fingerprints, aggregator/ATS title matching, and fuzzy matching. Duplicate
markers hide copies without discarding the source posting.

`CoverageGated` adapters receive a gate that condemns nothing: the company-coverage resolver
always returns no covered companies, and no posting is refused on title (#4412). Unreadable details keep the existing open-only liveness refresh and
withhold stale-close scopes when the unreadable share exceeds the normal threshold.

The `ats_covered` reason is historical for ordinary aggregator ingestion. LinkedIn native-board
discovery also uses it to retire unpublished discovery records; that separate publication
policy is not permission to reopen them as public LinkedIn vacancies.

For recovery, reopen only records closed for this reason, with recent actual source evidence
and no expired application deadline. Preserve last-seen timestamps and duplicate markers,
and enqueue the resulting state for search. Older records must be confirmed by a new crawl.
