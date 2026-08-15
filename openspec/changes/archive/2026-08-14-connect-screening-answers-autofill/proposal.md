## Why

The profile now has a screening-answers questionnaire (`internal/screeninganswers`: authorized
countries, visa sponsorship, desired salary, notice period, willingness to relocate, 18+), and
the server already folds those answers into the autofill profile the browser extension reads.
The agent-driven autofill path already grounds its plan in them. The extension's deterministic
fallback filler — the one that runs when the agent path is unavailable (no LLM configured, no
browser-tool channel attached, or the agent finds no fillable fields) — did not: its profile
type and its label-matching table both stopped at the nine identity fields, so a candidate who
relies on the fallback got a form with visa/salary/notice/relocation/age questions left blank
even though they had already answered them once in their profile.

## What Changes

- Extend the extension's `AutofillProfile` type with the six screening-answer fields the
  server already serves from `GET /api/v1/me/autofill-profile`.
- Map those six fields into the values the deterministic filler plans fills from.
- Add label-matching synonyms for the real-world ATS phrasings of each screening question
  ("which countries are you authorized to work in", "will you now or in the future require
  sponsorship", "desired salary", "notice period", "are you willing to relocate", "are you at
  least 18 years of age").
- Deliberately exclude a generic "are you authorized to work" Yes/No phrasing from the
  authorized-countries match: that question is boolean, the stored answer is a country list,
  and the profile carries no boolean field to answer it with — filling it would put the wrong
  shape of value into a Yes/No control.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `extension-autofill`: the deterministic fallback filler now also recognizes and fills
  screening-answer questions (authorized countries, visa sponsorship, desired salary, notice
  period, willingness to relocate, 18+), in addition to the identity contact block it already
  filled. The canonical profile block it reads from gains the corresponding six fields.

## Impact

- `extension/lib/freehire.ts` — `AutofillProfile` interface.
- `extension/entrypoints/sidepanel/App.svelte` — `profileToValues()`.
- `extension/lib/form.ts` — `FIELD_SYNONYMS`.
- `extension/lib/form.test.ts` — coverage for the new mappings and the deliberate exclusion.
- No backend change: `internal/screeninganswers` and the `/me/autofill-profile` /
  `/me/autofill/run` endpoints already served/consumed these fields before this change.
