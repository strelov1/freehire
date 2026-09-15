## Context

Confirmed live against `recruiterflow.com/radhires/jobs` (12 open postings, verified via
raw HTML inspection rather than static-endpoint guessing or a headless-browser network
capture — the listing was never an XHR call at all, so neither prior technique in this
initiative would have found it):

- The page inlines `<script>window.jobsList = {"department": [...], "group": [...],
  "location": [...]};</script>` — the SAME 12 postings presented three different ways
  (grouped by department, by "group" — confirmed byte-identical to `department` on the one
  sampled tenant, likely an alias — and by location). Only `department` is used; `group`
  and `location` are redundant regroupings of the identical dataset.
- `department` is `[[<name>, [<item>,...]], ...]` — an array of `[string, item-array]`
  pairs, not a plain object, so it needs a small tuple-shaped `UnmarshalJSON` (the same
  "decode a 2-element heterogeneous array" shape `schemaPlaces` already handles for a
  different reason).
- Each item: `apply_link` (a board-prefixed relative path, e.g. `"radhires/jobs/431"`),
  `details` (a free-text region/location list, e.g. `"LATAM - Argentina, LATAM - Brazil,
  ..."`, sometimes 20+ comma-separated entries), `employment_type` (confirmed live: `"Full
  time"`, `"Part time"`, `"Contract"` — all three, plain English), `job_id` (int),
  `job_name` (title), `last_opened` (RFC3339 with a non-colon numeric offset, e.g.
  `"2026-09-03T19:43:26+0000"`), `remote_type` (confirmed live: `"Remote"`, `"Hybrid"`, and
  JSON `null` — decodes cleanly into a plain Go `string` either way, since `encoding/json`
  leaves a non-pointer string field at its zero value for a `null` source, no custom
  unmarshaler needed here unlike `recrutei`'s `location` field).
- No `description` field anywhere in `window.jobsList` — each posting's own page
  (`recruiterflow.com/<board>/jobs/<id>`, the same URL `apply_link` names) carries a
  standard schema.org `application/ld+json` `JobPosting` block with the real, per-posting
  description (verified live: two different postings' descriptions are genuinely distinct
  prose, not a template placeholder).
- `hiringOrganization.name` in that same ld+json block is `"Rad Hires"` on every sampled
  posting — the agency's own name, never a per-posting end-client. RecruiterFlow is a
  recruiting-agency platform (the board IS the agency), so this is expected and requires no
  special handling, the same posture `huntflow.go`'s agency/hub boards already establish
  (no `aggregator()` marker either, on the same precedent).

## Goals / Non-Goals

**Goals:**
- Crawl a RecruiterFlow tenant's open postings from the single listing page's embedded
  `window.jobsList` (enumeration + location/employment-type/remote-type/date, all from the
  listing's own fields) plus one detail fetch per posting for the description only.

**Non-Goals:**
- No completeness cross-check against a declared total. Unlike `scalis`/`recrutei`/
  `pyjamahr`, `window.jobsList` carries no `count`/`total` field at all. The completeness
  argument here is structural rather than a runtime check: there is no pagination
  mechanism whatsoever — the whole board is embedded in one page load, so there is no
  "next page" a truncated read could ever miss. `fullBoardListing` still applies on that
  basis (the same "a single, unpaginated response IS the whole board" posture
  `geekhunter.go`'s ItemList already has, there additionally cross-checked against
  `numberOfItems` — a confirmation this adapter cannot perform, not a requirement it is
  missing).
- No `group`/`location` groupings used. Byte-identical to (or a redundant regrouping of)
  `department` on the one sampled tenant; parsing three copies of the same data for no
  behavioral difference would be pure overhead.
- No structured location parsing of `details`. It is a free-text, often very long,
  comma-separated region list (not a single city/country) — stored verbatim as `Location`
  rather than attempting to split or normalize it.
- No `Company` per-posting mapping. Neither `window.jobsList` nor the detail ld+json names
  anything but the agency itself; `CompanyEntry.Company` is used directly for every
  posting, the same posture every other single-tenant board in this initiative uses.

## Decisions

- **`fullBoardListing` applies on structural grounds, not a runtime completeness check** —
  see the Non-Goals entry above. A listing fetch or decode failure still fails the whole
  `Fetch`, and there is no partial-success path to guard against since there is no
  pagination to partially walk.
- **Employment type and work mode both come from the listing's own fields**
  (`employment_type`→`vocab.EmploymentTypeValues` via a small defensive plain-English map
  mirroring `humanbitEmploymentType`'s style; `remote_type` through the shared
  `workplaceTypeMode` helper, which already lowercases and already matches `"remote"`/
  `"hybrid"` verbatim) — no detail-page field is needed for either, unlike `recrutei`
  (whose detail-page `employmentType` turned out to be an unreliable constant) or
  `pyjamahr` (whose detail carries the only copy).
- **Only the description is read from the detail page**, via the shared `ldJobPosting`
  decoder — no new parsing machinery, only a struct selecting the one field this adapter
  needs.
- **A detail-fetch failure marks only that posting Unreadable, never the whole board** —
  the listing already proves the posting exists and names it; the same posture every other
  listing-then-detail adapter in this package gives (`humanbit`, `recrutei`, `geekhunter`).

## Decisions (recognizer)

- **No `internal/ingest/atsboard` entry is added for `recruiterflow.com`.** Confirmed
  live: the bare apex domain also serves the platform's own marketing site
  (`recruiterflow.com/pricing`, `/blog`, and others all answer HTTP 200 — not a 404), and
  every existing recognizer mechanism (`modePath`'s plain first-segment rule,
  `reservedSegments`, `noBoardFirstSegments`) either reads a marketing path word as a
  board or requires enumerating every such word in advance, which is an incomplete,
  fragile denylist rather than a real fix — a future marketing page not yet in the list
  would still misrecognize. Every real tenant URL's SECOND segment is always `jobs`
  (`<board>/jobs` or `<board>/jobs/<id>`), which no existing atsboard mode can require;
  adding one would be new shared-recognizer infrastructure built for a single adapter's
  edge case, not a one-line table row. A future contribution of this platform lands in
  `board_submissions` for manual triage instead of self-resolving — the same accepted
  cost the Oracle Fusion HCM recognizer gap already carries (see
  board_submissions_leads_2026-09-14 project memory) rather than risk polluting `boards`
  with junk.

## Risks / Trade-offs

- [No completeness cross-check, unlike the prior three adapters in this initiative] →
  accepted per the Non-Goals reasoning above; revisit if a future, larger tenant's
  `window.jobsList` turns out to carry a `count` field this one sampled tenant simply
  didn't exercise.
- [`window.jobsList` is an undocumented internal frontend variable, not a published API] →
  the same posture this codebase already accepts for `recrutei`/`pyjamahr`'s equivalent
  internal APIs and the RSC-flight-based adapters: it could change without notice.
- [Only one tenant confirmed (`radhires`, 12 postings)] → the employment-type/remote-type
  vocabularies are unusually well-confirmed for a single tenant (three and three values
  respectively, all plain English), but a future tenant could still expose an unseen
  spelling; the defensive mapping style already covers the plausible siblings.

## Migration Plan

No data migration. Ship the adapter, merge, deploy, then add `recruiterflow/radhires` by
hand via `cmd/add-board` to close `board_submissions` id 26.
