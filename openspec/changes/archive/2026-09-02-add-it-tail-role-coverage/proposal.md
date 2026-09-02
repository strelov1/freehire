## Why

Measured against prod on 2026-09-02: of 1 971 625 open postings in the 60 000
largest title groups, **935 604 (47.5%) reach the search index with an empty
`roles` array** — unfilterable by role. Every single one of them has an empty
`category`: not one posting exists where the category resolved and the role did
not. The bottleneck is `classify`, not `roletag`, and the fix is title aliases.

Inside that gap, **20 492 open postings across 836 title groups are recognisably
IT work** the dictionary simply has no word for. Two findings stand out:

- The whole Russian software vocabulary is missing. `Программист` (692),
  `Инженер-программист` (737+78), `Техник-программист` (95), `Системный
  администратор` (566), `Администратор баз данных` (71) resolve to nothing —
  and so do `Java-разработчик` and `Python-разработчик`, because bare
  `разработчик` is not an alias at all.
- `Systems Engineer` (1440) plus its spellings is the single largest unresolved
  IT title in the catalogue.

This change takes the unambiguous part of that 20 492. The ambiguous
remainder — `Automation Engineer`, bare `Application Engineer`, the SAP and
Dynamics functional consultants — is deliberately left for the industrial
wave, where it has a category to go to.

## What Changes

- Add the Russian software vocabulary: `программист`, `разработчик`,
  `инженер-программист`, `техник-программист`, `системный администратор`,
  `сетевой администратор`, `администратор баз данных`.
- Resolve the `Systems Engineer` family, with the non-IT lookalikes
  (`Control`/`Power`/`Electrical`/`Quality Systems Engineer`) declared blind
  ABOVE the bare alias so they keep resolving to nothing rather than being
  swept into software.
- Resolve the vendor-platform titles that name a product and therefore a
  discipline: ServiceNow, Salesforce, Oracle DBA, SharePoint, Mainframe,
  Tableau.
- Resolve the infrastructure and IT-support titles: Data Center
  Technician/Engineer, Release Engineer, Cloud Operations/Migration Engineer,
  Network Operations Engineer, Network Specialist/Technician, IT
  Specialist/Technician, the Integration Engineer family.
- Add the named roles those titles deserve: `salesforce_developer`,
  `sap_developer`, `servicenow_developer`, `systems_engineer`, and fix
  `systems_administrator`, which today is reachable only through the PLURAL
  spelling — "System Administrator" gets the category but no role.

Not breaking: every alias added lands on a title that resolves to nothing
today, and each carries a regression test naming the title it must NOT take.

## Capabilities

### New Capabilities
- `it-title-coverage`: the IT titles the catalogue carries that the category
  dictionary had no word for — the Russian software vocabulary, the Systems
  Engineer family and its non-IT lookalikes, the vendor-platform titles, and
  the infrastructure/support tail — plus the named roles they expose.

### Modified Capabilities
<!-- None. `role-category-alias-coverage` states the rule these aliases follow
     ("New aliases never steal from a more specific existing alias") and keeps
     it unchanged; this change adds instances under its own capability rather
     than editing that one's requirements. -->

## Impact

- `internal/dict/classify` — the title `categoryTable`, which resolves in
  DECLARATION ORDER, so placement is the design.
- `internal/dict/roletag` — new named-role entries plus the singular
  `system administrator` alias.
- `cmd/gen-contracts` output (`web/src/lib/generated/contracts.ts`). No new
  CATEGORY value, so `labels.ts`, `filterSections.ts` and
  `extension/lib/labels.ts` are untouched — this change adds no category.
- Rollout: `backfill-derive` then a plain `make reindex`. Roles are derived at
  index time and need only the rebuild.
- Cost: the newly-categorised postings become `is_tech = true` where the
  category is technical, so they enter the enrichment queue — bounded by the
  ~20k figure above, and smaller in practice since this change takes only part
  of it.
