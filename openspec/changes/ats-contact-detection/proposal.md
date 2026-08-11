## Why

The ATS report tells candidates who already have an email in their CV to "Add both an email and a phone number near the top", and docks them 4 points for it. Reproduced against a real CV: the email and LinkedIn patterns both match, and the phone does not — the number is written as a bare 11-digit national mobile (no `+`, no separators, no parentheses), which none of the phone pattern's three branches accept. The pattern only recognizes `+`-prefixed international numbers and two US shapes (`(555) 123-4567`, `555-123-4567`), so candidates outside those conventions are told their CV lacks a phone it plainly contains.

Two independent defects compound into misleading advice: the detector misses valid numbers, and the line item then misreports *which* detail is missing because it ANDs two separate signals into one check.

## What Changes

- Broaden phone detection to accept national and unseparated formats — a digit run within a plausible phone length, with optional separators, parentheses and country prefix — instead of only `+`-prefixed and two US shapes.
- Guard the broader pattern against false positives that a résumé is full of: four-digit years, year ranges, dates, salary and metric figures, and long identifiers.
- **Split the combined contact check into two line items**, one for email and one for phone, so the report names the detail that is actually absent and awards the points that were earned. The combined item cannot say which half failed, which is what produced the wrong advice.
- Keep the total contribution of contact information to the score unchanged, so existing scores do not inflate.

## Capabilities

### New Capabilities

- (none)

### Modified Capabilities

- `cv-ats-score`: the deterministic score's contact check becomes two independently scored items, and phone presence is recognized for international, national and unseparated number formats rather than `+`-prefixed and US-style ones only.

## Impact

- `internal/atscheck` — the phone pattern and `formatCategory`'s item list; the score's item ids and labels change shape, so anything keyed on the single contact item id must follow.
- `internal/atscheck` tests — new cases for the formats that currently fail, and for the false positives the broader pattern must still reject.
- The report is recomputed per request and nothing is persisted, so no migration and no backfill. A cached LLM review is unaffected: it is merged on top and does not carry these items.
- Candidates whose phone was previously undetected gain the phone item's points; nobody loses points, because detection only widens.
