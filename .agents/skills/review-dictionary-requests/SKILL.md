---
name: review-dictionary-requests
description: Review Freehire dictionary requests from /dictionary, classify duplicates and ambiguous terms, implement useful vocabulary additions, and reconcile request statuses when authorized.
---

# Review dictionary requests

Work from the repository root. The public `/dictionary` page describes the vocabulary;
submitted requests live in production `dictionary_gaps`, not on that page.

## Inspect the queue

Read root AGENTS.md for current production access, `.out-of-scope/README.md`, and the
AGENTS.md of the requested dictionary. Inspect the schema in
`migrations/0188_dictionary_gaps.sql`, later migrations affecting it, and
`internal/platform/db/queries/dictionary_gaps.sql` before querying.
Use the established SSH/Postgres access; keep credentials out of output.

Fetch every pending row, including id, kind, raw_term, normalized_term,
request_count, first_requested_at, last_requested_at and resolution_note.
Fetch status totals separately. Dates are request dates, not proof of when a
human last reviewed the queue. Avoid exposing submitter identities.

Complete the inspection with one disposition for every pending row:

- Already supported: identify the canonical and verify actual canonicalization.
- Add alias: existing concept, new spelling; preserve the canonical slug.
- Add concept: a distinct, useful skill, specialization or company industry.
- Needs clarification: multiple meanings, combined concepts, or insufficient evidence.
- Reject: test noise, wrong vocabulary kind, or a documented exclusion that still applies.

Search dictionary mappings, descriptions, ambiguity gates, tests and exclusion
comments. A string match alone does not prove a term resolves. Preserve the
separation between skill, role, seniority and company industry. Broad occupational
skills can be valid; being nontechnical alone is not grounds for rejection.
Consult primary product documentation when a product's identity or scope is uncertain.

## Implement authorized additions

A request to inspect or discuss produces a proposed disposition. A request to add
useful terms authorizes the corresponding repository edits. Keep uncertain items
pending and explain the specific ambiguity rather than inventing aliases.

For skills, follow `internal/dict/skilltag/AGENTS.md`: alias mappings, display labels
where mechanical title casing is insufficient, and descriptions belong together.
Use the existing thematic dictionary tables. After adding canonical skills, read
`internal/dict/skillvec/AGENTS.md` and run `go generate ./internal/dict/skillvec/`
to append their permanent vector positions; preserve existing indices. Read the corresponding owners for
specializations and industries instead of treating them as skills.

Choose canonical spellings deliberately. Correct a submitted typo into its actual
concept; a one-off typo need not become a permanent prose alias. Check whether an
alias is a common verb or ordinary phrase: `design systems` also occurs in
"design systems that scale". Respect intentional exclusions and the distinction
between structured-list canonicalization and prose parsing. New strong phrase
matches can also corroborate nearby ambiguous skills; inspect that effect.

Test canonicalization and realistic prose for the accepted spellings. Add negative
examples when ambiguity warrants them. Run the affected dictionary tests and the
repository's applicable checks, including Go vet and integration-tag compilation.
Review only this task's diff; preserve unrelated work.

## Reconcile only what is live

Code prepared locally is not a deployed dictionary. Keep new requests pending until
production resolves them. Deployment and catalogue backfill are separate actions:
follow the user's authorized scope and the worker/deployment instructions before
running either. Existing job facets need backfill and search refresh as documented
by the dictionary owner.

When queue updates are authorized, close existing supported terms as accepted,
and clear confirmed test noise or unsupported requests as rejected with an English
resolution_note. For new additions, verify live resolution before acceptance.
Use a transaction and explicit reviewed ids with `WHERE status = 'pending'` and
`RETURNING`; compare changed rows with the intended set before committing. Preserve
request counts and timestamps. Re-read the affected rows after committing.
Accepted rows remain live under a partial unique index; rejected terms may be
submitted again. Do not delete request history or bulk-close the pending queue.

Report added concepts and aliases separately, closed requests, items still pending,
checks actually completed, and whether the result is local or deployed. Link the
changed artifacts. Do not claim acceptance, deployment or backfill from local tests.
