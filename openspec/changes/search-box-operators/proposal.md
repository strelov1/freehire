## Why

The header search box offers only free text plus picked suggestions. A visitor who knows
exactly what they want — one employer, one source, a title and nothing else — has to find
the right suggestion row or open the filter panel. A small typed syntax (`title:`,
`company:`, `source:`) lets them say it directly, and contextual suggestions after the
colon make the syntax discoverable rather than something to memorise.

## What Changes

- The header search box recognises three operators in the typed text: `title:<value>`,
  `company:<value>`, `source:<value>`. A value is one word or a `"quoted phrase"`.
- On Enter the operators become the filters the app already has: `company:` → the
  `company_slug` facet, `source:` → the `source` facet, `title:` → a quoted `q` with
  `q_fields=title`. They apply as chips; nothing new reaches the URL or the API.
- While the caret sits in an operator's value, the dropdown suggests only that kind:
  companies after `company:`, sources after `source:`, title completions after `title:`.
  Picking one writes the canonical value back into the box.
- No backend change. Every target filter already exists.

## Capabilities

### New Capabilities

- `search-box-operators`: typed field operators in the header search box and their
  contextual suggestions.

### Modified Capabilities

(none)

## Impact

- `web/src/lib/` — a new pure parser module and its tests; `HeaderSearch.svelte` wiring.
- Reads existing endpoints only: `/api/v1/companies`, `/api/v1/suggest`, and the `source`
  facet distribution.
