# Separate job application destination

Keep `jobs.url` as the source posting used for identity and liveness. Store a verified
employer application destination in `jobs.application_url`, empty when unknown. Public
job URLs and per-city Apply links prefer that destination and otherwise retain the
source URL. Optional public `source_url` preserves original-posting and attribution links
when Apply points elsewhere. On-site applications retain their existing behavior.

The user approved this separation and the verified Gist ATS destination. Guest LinkedIn
pages currently hide that destination behind sign-in, so this change does not promise
automatic extraction or introduce account-based crawling. A curator command validates
and records known destinations, previews by default, supports clearing them, and queues
the changed job for search in the same transaction. Ingest does not overwrite the field.
Verified destination lookup precedes source-page canonicalisation and preserves vacancy
query parameters and form paths, ignoring only known tracking (`utm_*` and `trk`).

## Approaches

Replacing the source URL would break LinkedIn liveness and lose provenance. A new
resolver service or authenticated crawler would add a separate collection mechanism.
The selected approach adds one optional destination to the existing posting model.

## Acceptance

- Gist's Apply opens its verified HR Department posting while its stored source URL and
  source remain LinkedIn.
- Unknown destinations continue opening the original source posting.
- Re-ingest cannot erase a recorded destination; setting or clearing one queues search.
- Existing on-site applications keep their empty outbound URL.
