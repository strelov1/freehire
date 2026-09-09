## Context

See `proposal.md` - Why. Two relevant facts about the existing code:

- `internal/identity/username` already owns exactly the sanitisation and collision-suffix
  rules a mentor slug needs: `Sanitize(s)` (lowercase, `.`→`-`, everything else outside
  `[a-z0-9-]` dropped, hyphens collapsed/trimmed, truncated to 30, falls back to `"user"`
  below 3 chars) and `Candidate(base, n)` (`base` for n<=1, else `base-n`). It has no
  dependency beyond `strings`/`regexp`/`strconv` - importing it from `mentorship` adds no
  new service wiring, and `identity` (layer 3) sits below `engage` (layer 7), so the
  import is layering-legal.
- `internal/engage/mentorship/profile.go:95-97` already documents that an edit ignores
  `slug` and `company_slug` entirely - `UpdateMentorProfile`'s SQL doesn't touch either
  column. That immutability is intentional (stable shared links; a company change is a
  new claim needing re-moderation) and this change does not touch it.
- `CompanyPicker.svelte` already exists and already does exactly what a company field
  here needs (typeahead over `api.listCompanies`, returns `{slug, name}`); it's used
  today in `ReferralsView.svelte` for the same "don't make the user guess a slug" reason.

## Goals / Non-Goals

**Goals:**
- A blank "Your URL" at submission time always yields a valid, unique slug without a
  round trip to the user.
- An explicit slug typed by the mentor keeps today's validation and collision behavior
  unchanged (still refused outright, never silently suffixed).
- The company field in the creation form is chosen by name, not typed as a slug.

**Non-Goals:**
- Editing `slug` or `company_slug` after creation - explicitly out of scope, see
  proposal.md.
- Any change to `CompanyPicker.svelte`'s own behavior or its use in `ReferralsView`.
- A redirect/alias mechanism for a changed slug - moot since slugs stay immutable.

## Decisions

**Derive the fallback base from `DisplayName`, not from the account's `username`.**
`DisplayName` is already a required field on this exact form, so no new dependency is
needed. Reaching for the account's `username` instead would require `mentorship.Service`
to depend on `internal/identity/accounts` (a new constructor argument and a new wiring
edge in `cmd/server`) for a "nicer" default that the mentor can't see anyway before they
finish filling the form - not worth the coupling. `DisplayName` also degrades the same
way `username.Sanitize` already degrades elsewhere in the codebase (a name with no
latin/digit characters falls back to `"user"` plus a suffix), so this introduces no new
failure mode.

**Retry only a derived slug, never an explicitly supplied one.** If a mentor typed a
specific address and it's taken, silently handing them a different one (`chosen-2`)
would be surprising and undiscoverable - they'd need to notice the URL differs from what
they typed. A derived slug carries no such expectation, so retrying it is invisible in
the way a good default should be. `Service.SubmitProfile` tracks whether the slug came
from the caller (kept only for a taken-slug 409) or was derived (retried up to
`maxSlugAttempts`, mirroring `accounts.maxUsernameAttempts`'s reasoning: a pathological
run of collisions should fail loudly rather than loop forever).

**Frontend live-derives the preview independently of the backend, not via a request per
keystroke.** `username.Sanitize`'s rule is small and pure; reimplementing it in
TypeScript avoids a network round trip on every keystroke of the name field for a value
that's only ever a preview - the backend remains the sole authority on the slug actually
stored, and the preview only needs to look right, not be authoritative. The field stays
a plain editable input (not a disabled preview) so a mentor who wants a specific address
still types one.

**Company field: swap in `CompanyPicker` for create mode only; edit mode shows plain
text.** Reusing the existing component costs nothing new. For an existing profile,
`company_slug` can't change (see Context), so a picker there would look editable while
silently doing nothing on submit - worse than the current disabled input. Rendering the
company name as static text for an existing profile matches what's actually true: it's
not a form field anymore, it's a fact about the profile.

## Risks / Trade-offs

- **A frontend sanitiser drifting from the backend's `username.Sanitize`** would make the
  live preview lie about what gets stored. Mitigation: keep the TS version intentionally
  minimal (the same four rules: lowercase, drop disallowed chars, collapse/trim hyphens,
  truncate) and note in its own comment that it must mirror `username.Sanitize`; the
  backend re-derives from `DisplayName` itself rather than trusting whatever the client
  sent as a "preview" value entangled with the real slug field, so a stale preview can
  at most show the wrong suggestion, never store the wrong slug.
- **`maxSlugAttempts` exhausted** (extremely unlikely - would need 100+ mentors sharing
  the same sanitized display name) surfaces as `ErrSlugTaken`, which the existing handler
  already maps to a 409; a mentor hitting this can still type an explicit slug.
