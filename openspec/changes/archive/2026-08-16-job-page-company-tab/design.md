## Context

`JobView.svelte` renders a job in two columns: a sticky sidebar of salary, actions and
facets, and a content column holding the model-written summary and the description. The
company appears only as a name, a logo and a link in the row above the title
(`JobView.svelte:230`).

Everything the Company tab needs already exists:

- `GET /api/v1/companies/:slug`, reached through `api.getCompany(slug, limit, offset)`,
  returns `{ company, jobs, referral_available }`.
- `CompanyFacts.svelte` and `CompanyAbout.svelte` render a company's scalar facts and its
  full description. Both are *present-only*: given a company with nothing to show they
  render no markup at all, so neither can leave an empty box behind.

The repo has previously been bitten by two things this change walks straight into, both
recorded in `GhostChecklist.svelte`'s history: wrapping an `aria-controls` target in
`{#if}` leaves a dangling IDREF on first paint, and the HTML `hidden` *attribute* loses
to a Tailwind display utility because the two have equal specificity and utilities are
emitted later in the stylesheet.

## Goals / Non-Goals

**Goals:**

- Let a visitor read who the employer is without leaving the posting.
- Reuse the company page's rendering rather than growing a second one.
- Keep the company's copy out of the job page's crawlable HTML.

**Non-Goals:**

- No backend work. No new endpoint, no new field on the job wire shape.
- No URL for the tab. The tab is page state, not a route.
- No company job count in the panel. `Company` in `web/src/lib/types.ts` does not carry
  `job_count` (only `CompanyListItem` does), and adding it to render one label is not
  worth the wire change.
- No change to the sidebar, the header row, or the existing company link.

## Decisions

### The tab is page state, not a route

The job page's other secondary surfaces are sub-routes: `/jobs/[slug]/fit`,
`/copies`, `/discussion`. The Company tab deliberately is not.

A sub-route would need `noindex, follow` and a canonical back to the posting — the
treatment `copies/+page.svelte` already carries — because its content is a duplicate of
`/companies/<slug>`. A thin duplicate page that must be excluded from the index is a page
that should not exist. Local state costs nothing to exclude.

The cost is that the tab is not linkable or restorable across a reload. For a panel whose
content has a canonical home one click away, that is an acceptable loss.

*Alternative considered:* a sub-route with `noindex`. Rejected — more moving parts, and a
crawlable URL we would then have to spend effort keeping out of the index.

### Fetch on first activation, cached for the visit

`api.getCompany(slug, 1, 0)` is called the first time the tab is activated, and the
resolved company is held in component state. `limit=1` is the smallest fetch the endpoint
allows: the API clamps `limit` to at least 1 in `pageParams`, so a zero-job request is not
expressible. The one returned job is discarded. This is the same compromise
`web/src/routes/companies/[slug]/+page.server.ts` already documents, and it is a note for
the same future backend company-entity-only path, not a new debt.

*Alternative considered:* prefetch on hover. Rejected as premature — the endpoint is fast
and a click already feels immediate; hover prefetch spends requests on every passing
cursor.

### The strip is the design system's `TabStrip`; both contents stay mounted

`TabStrip` (`design-system/src/tab-strip.svelte`) already owns everything a hand-rolled
row would have to re-earn: `role="tablist"`, a roving tabindex, arrow-key and Home/End
movement, and an overflow-scrolling row that degrades on a narrow viewport. It is the
component `/my/profile` uses. Writing a second tablist here would mean a second copy of
that keyboard contract, and the two would diverge.

`TabStrip` points every tab's `aria-controls` at ONE panel id, which the call site owns.
That is what disarms the `aria-valid-attr-value` failure the repo hit in
`GhostChecklist.svelte`: the panel element is unconditional, so the IDREF cannot dangle.
The id comes from `$props.id()` — Svelte's SSR-stable per-instance primitive — rather than
a hardcoded string, so a second `JobView` on one page cannot claim the same panel. Note
`$props.id()` is only legal as a variable declaration's initializer; it cannot be
interpolated inline.

Inside that panel, the two contents are toggled with `class={active ? 'block' : 'hidden'}`
rather than `{#if}`. Unmounting the company content would discard a fetch the visitor
already waited through, and re-render the description on every switch back. The `hidden`
*attribute* is deliberately not used: `[hidden] { display: none }` from preflight and a
Tailwind display utility have equal specificity, and the utility is emitted later, so the
utility wins and the element stays visible.

Keeping the description mounted has no SEO cost: it is the default-selected content and
is in the server-rendered HTML either way.

### The panel is keyed on the company slug

`JobView` is not remounted when the route parameter changes, so a client-side navigation
to another job would otherwise leave the previous company's data in the panel. The panel
is wrapped in `{#key companySlug}`, the same treatment `VoteControl` already gets in this
file for the same reason.

### Panel states

`idle → loading → (loaded | empty | error)`.

`empty` is decided after the fetch, by asking whether the company has anything worth
showing. It cannot be known before the fetch, which is why the tab is offered
unconditionally to any job with a company slug and the emptiness is reported inside the
panel rather than by withdrawing the tab. Withdrawing it would collapse the strip under
the visitor's cursor immediately after their click.

`error` leaves the rest of the page untouched; the panel reports the failure and offers
the link to the company page, which is the content the visitor wanted anyway.

### The present-only conditions move into a pure module

`CompanyFacts` and `CompanyAbout` each decide internally whether they have anything to
render — the facts/badges lists and the trimmed description are `$derived` inside the
components. The panel's `empty` state has to ask the same question, and a second copy of
those conditions would drift: the panel would announce details, both cards would render
nothing, and the visitor would get a heading over a void.

So the derivations move to a new `web/src/lib/companyDetails.ts` — `companyFacts`,
`companyBadges`, `companyDescription`, and a `hasCompanyDetails` predicate composed from
them. Both cards consume it, and the panel asks `hasCompanyDetails`. One definition, one
place to change.

This also puts the feature's only real logic somewhere it can be tested. `web`'s vitest
runs in `environment: 'node'` with no Svelte compilation (`web/vitest.config.ts`), so a
component cannot be rendered in a test — but a pure module can, and the repo already
keeps company logic this way in `companyFacetModel.ts` beside `companyFacetModel.test.ts`.
Everything else in this change is markup and wiring, verified by `svelte-check` and by
inspection in a real browser.

## Risks / Trade-offs

- **A visitor clicks Company and gets "we don't have details yet".** → Unavoidable
  without a per-job flag on the wire shape, which is a backend change this feature does
  not justify. The panel always offers the link onward, so the click is never a dead end.
  If the empty rate turns out to be high in practice, the fix is a boolean on the job
  projection, and the seam for it is the panel's `empty` state.

- **The tab is not restorable across reload or shareable.** → Accepted, per the routing
  decision above. `/companies/<slug>` is the shareable surface.

- **`limit=1` fetches a job nobody uses.** → One row, already the endpoint's floor.
  Documented at the call site so it is not mistaken for carelessness.

- **The strip changes the content column's vertical rhythm on every job page.** → Verified
  visually against both a long and a short description before merge, plus the no-company
  case where the strip must be absent entirely.

## Open Questions

None.
