## Context

`JobCompanyPanel.svelte` renders whatever `$lib/companyDetails` derives from a company.
That module was extracted in the change that introduced the tab, precisely so a second
surface could not disagree with the company page about what a company has.

The data was already there. `companyView.CompanyInfo` is a `json.RawMessage` on the Go
side, so the whole stored blob reaches the browser; the front-end `CompanyInfo` interface
declared roughly a third of it. Nothing about this change touches the API.

## Goals / Non-Goals

**Goals:**

- Answer "who runs this, where are they, where else can I read about them" inside the tab.
- Keep the additions inside the existing derive-then-render split.

**Non-Goals:**

- No backend work, no new endpoint, no migration.
- No `tech_stack`. See the proposal — the stored list is a scanner's inventory, not the
  company's claim, and rendering it verbatim would mislead. Its useful form is the
  intersection with `internal/skilltag`'s dictionary, which is a separate change.
- No `benefits`. Present for 21 of 40, but each entry is a `{title, category}` pair of
  marketing copy ("Growth by way of learning"), and a job page is the wrong place to
  reprint it.

## Decisions

### Link schemes are allow-listed, not sanitised

`companySocials` puts each stored value through `new URL()` and keeps it only if the
protocol is `http:` or `https:`.

This is the one genuinely load-bearing rule in the change. The values are written by an
external importer into a field nobody validates, and they end up in an `href`. A
`javascript:` URL there is script execution on our origin — the classic form of this bug.
An allow-list fails closed: an unparseable or unexpected value renders nothing, which
costs a visitor one icon and costs an attacker everything.

Protocol-relative `//host` is refused as a side effect worth naming: `new URL()` cannot
parse it without a base, and that ambiguity is exactly what should not reach an anchor.

*Alternative considered:* strip dangerous prefixes. Rejected — a deny-list of schemes is a
guessing game (`javascript:`, `\tjavascript:`, `JaVaScRiPt:`, `data:`, `vbscript:`), and
the browser's own parser already answers the question exactly.

### Offices get their own row, not a fifth column

The facts grid is four fixed columns. Meta records 35 office countries; Stripe 11. Neither
fits a grid cell.

`CountryFlagStack` already solves this — it is the overlapping cluster the job sidebar uses
for a many-country remote role, capped with a "+N" chip. Reusing it costs nothing and keeps
one flag treatment across the site. The cap here is 12 rather than the default 6, because
the panel is far wider than the sidebar it was written for.

`link` is deliberately off. The stack's link mode points at the jobs filter for that
country, which would promise roles the employer may not have open there. An office is not
a vacancy.

### Brand marks go in the design system

`ProviderIcon` exists because lucide ships no brand logos, and it already holds LinkedIn,
GitHub, Google, Telegram, Apple and Discord. X, Facebook and Instagram belong beside them,
not inlined in a page component.

`twitter` and `x` both select the X mark. The stored field is still named `twitter`, and
making every caller translate the name before asking for an icon would be churn for its
own sake.

Website uses lucide's `Globe` instead — a generic destination has no brand.

### The CEO is a fact, not a section

It is one short string. `companyFacts` already produces the ordered term/value list both
the tab and the company page's sidebar card render, so adding a row there puts the CEO on
both surfaces at once, in one place.

Position: after Headquarters, before Type. "Who runs it" is more interesting to a
candidate than the legal organisation type it sits in front of.

## Risks / Trade-offs

- **A company's recorded links go stale or 404.** → We do not own them and cannot verify
  them at render time. `nofollow` means a dead destination costs us nothing beyond one
  broken click, and the tab's link to the company page is unaffected.

- **`locations` is free-form enough to carry names, not just codes.** → Anything that is
  not a two-letter code is dropped rather than guessed at, so a flag component is never
  handed something it would render as a blank.

- **Adding a `ProviderIcon` consumer moves the design-system adoption baseline.** → Known
  and recorded in the same commit; the gate fails on improvement as well as regression.

## Open Questions

None.
