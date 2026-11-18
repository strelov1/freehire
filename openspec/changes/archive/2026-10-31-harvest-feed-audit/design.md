## Context

The user approved a harvest extension after live feed research. The normal command discovers new boards, excludes known ones, then probes liveness. Feed audits need known boards and full descriptions. Current Scalis and Teamtailor adapters already accept the shared transport, allowing a comparison without changing ingestion.

## Goals / Non-Goals

Provide reproducible evidence for parser migrations. Start with the two verified existing providers. Generic endpoint guessing, automatic switching, new-provider ingestion and database persistence are outside this change.

## Decisions

Use a separate CLI mode with existing transport/egress and pacing. With a seed file, read only its boards and require no database; otherwise read the provider's current catalog. Reject `--apply`. Audit at most 20 boards by default, in deterministic order, sequentially, with a configurable positive deadline per board (`-audit-timeout`, default two minutes) and a default one-request-per-second limit. Explicit positive `-pace` remains available.

Decode only documented provider paths. Scalis custom XML groups rows by canonical vacancy URL and derives the original ID from its path; location-expanded reference values are not identities. Teamtailor RSS pages in batches of 500 to a short page, with a hard ceiling of 20 pages and duplicate-ID detection. Validate roots, required containers, native identities and consistent bodies rather than accepting an arbitrary XML response.

Represent each observation as canonical ID, title, URL, normalized body text and field-presence flags. Compare full ID sets and matched bodies; report per-field presence counts and losses on matched IDs. Presence is not value equivalence, and body equality is not evidence about unseen boards. Preserve this distinction in the report/documentation.

Run the real adapter with a counting transport for the baseline. An adapter can swallow detail errors, so any observed request failure makes the comparison unverified even if Fetch returns jobs. Feed 404/410 is unavailable; malformed XML, timeout, 403/429 and incomplete pagination are failures, never empty feeds. A failed baseline preserves feed evidence but yields no equality claim. Reports do not contain full descriptions or recruiter details.

## Risks / Trade-offs

Full baseline crawls cost requests; limits, pacing and deadlines keep an audit deliberate. Inventory can change between two reads, so differences are observations rather than proof of a bug. A valid empty feed says only that the feed is empty. Missing publication dates in Scalis and unavailable feeds on some Teamtailor hosts are reportable results, not reasons to discard the evidence.
