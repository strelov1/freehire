## Context

The extension already has two autofill paths sharing one server-assembled profile block
(`GET /api/v1/me/autofill-profile`, `internal/handler/autofill_profile.go`): an agent-driven
path (`POST /api/v1/me/autofill/run`) that already grounds its plan in screening answers, and
a deterministic fallback filler (`extension/lib/form.ts`) that label-matches question text
against a fixed synonym table (`FIELD_SYNONYMS`) and writes values from a flat `Record<string,
string>` (`extension/entrypoints/sidepanel/App.svelte`'s `profileToValues`). The server side of
this wiring predates this change; only the extension's TS type, value mapping and synonym
table were missing the six screening fields. See proposal.md - Why.

## Goals / Non-Goals

**Goals:**
- Extend the existing three extension-side seams (type, value mapping, synonym table) to
  carry the six screening fields through the same mechanism identity fields already use.
- Preserve the existing rule that an unstated field is left blank, never guessed.

**Non-Goals:**
- No backend change — `internal/screeninganswers` and both autofill endpoints already serve/
  consume these fields.
- No change to the agent-driven autofill path, which already has this data.
- No attempt to make the label-matcher fuzzy or ML-based; it stays the same
  front-anchored-prefix synonym match the identity fields use, just with more entries.

## Decisions

- **Reuse `FIELD_SYNONYMS`/`matchFieldKey` rather than a parallel table.** The six new
  fields are conceptually the same kind of thing the existing nine are: one canonical key,
  a handful of real-world label phrasings, one flat value. A second mechanism for "screening"
  fields would duplicate `planLabelFills`/`fillByLabel` for no behavioral gain.
- **Do not map a generic "are you authorized to work" Yes/No question to
  `authorizedCountries`.** The stored answer is a joined country-code list
  (`screeninganswers.AutofillFields` — "US, DE"), not a boolean. `fillField`'s checkbox/radio
  path only recognizes `"yes"`/`"true"`/`"1"` for a single control, and a grouped Yes/No radio
  pair matches by comparing the value against each option's own label — neither path can turn
  a country list into a Yes/No answer. Synonyms are restricted to phrasings that literally ask
  *which* countries ("which countries are you authorized to work in"), matching the existing
  `extension-autofill` spec's Roku-derived test case, which asserts the boolean phrasing stays
  unmatched.
- **Values arrive pre-formatted from the server, except authorized countries.**
  `screeninganswers.AutofillFields()` already renders booleans as `"yes"`/`"no"` and composes
  the salary string, so the extension routes those straight through. Authorized countries is
  the one exception: the server serves comma-joined ISO codes ("US, DE"), but the checkbox
  groups that question is almost always rendered as offer full country names ("United
  States") as their options, and `chosenOptions` (`extension/lib/form.ts`) matches the value
  against each option's own label text. Left as codes, the value would never match any
  option and the group would silently stay unchecked — caught in review, not in the original
  pass. `formatAuthorizedCountries` converts each code via the same `countryLabel`
  (`extension/lib/labels.ts`, `Intl.DisplayNames`) the extension already uses for job-facet
  country codes, so the two matching sides agree on country naming.

## Risks / Trade-offs

- [Front-anchored prefix matching misses some real ATS phrasings the identity fields don't
  already cover as densely] → Accepted: this is the existing mechanism's known shape (see
  `matchFieldKey`'s doc comment), and it degrades to "left blank" rather than a wrong answer.
  Widening coverage further is a follow-up, not a blocker for connecting what already exists.
