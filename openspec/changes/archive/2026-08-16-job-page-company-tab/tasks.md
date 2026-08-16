## 1. Present-only conditions as a pure module

- [x] 1.1 Write `web/src/lib/companyDetails.test.ts` first: `companyFacts`,
  `companyBadges`, `companyDescription` and `hasCompanyDetails` over a company with
  everything, a company with nothing, and the partial cases in between (facts but no
  description, description but no facts, badges only, whitespace-only description).
- [x] 1.2 Add `web/src/lib/companyDetails.ts` until those tests pass, lifting the
  derivations out of `CompanyFacts.svelte` and `CompanyAbout.svelte` unchanged.
- [x] 1.3 Rewire `CompanyFacts.svelte` and `CompanyAbout.svelte` to consume the module so
  there is one definition of "has anything to show", not three.

## 2. Company panel

- [x] 2.1 Create `web/src/lib/components/JobCompanyPanel.svelte` taking a company slug and
  name, with the `idle → loading → (loaded | empty | error)` state machine and a single
  `api.getCompany(slug, 1, 0)` call fired on first activation and cached thereafter.
  Document at the call site why `limit=1` and why the returned job is discarded.
- [x] 2.2 Render the loaded state: `CompanyFacts` and `CompanyAbout`, followed by an
  "All jobs at <name> →" link resolving to `/companies/[slug]`.
- [x] 2.3 Render the loading skeleton, the empty state ("We don't have details on <name>
  yet." plus the link) and the error state ("Couldn't load company details" plus the
  link), choosing `empty` via `hasCompanyDetails`.

## 3. Tab strip on the job page

- [x] 3.1 In `web/src/lib/components/JobView.svelte`, wrap the content column's existing
  summary and description in a `Description` panel and add the `Company` panel beside it,
  rendering the strip only when `job.company_slug` is set.
- [x] 3.2 Wire the tab semantics: `role="tablist"`/`role="tab"`/`role="tabpanel"`,
  `aria-selected`, `aria-controls` and `id` pairs from `$props.id()`, and left/right
  arrow-key movement between tabs.
- [x] 3.3 Toggle panel visibility with the Tailwind `hidden`/`block` utilities, keeping
  both panels mounted — not `{#if}`, and not the `hidden` attribute. Key the company panel
  on the company slug so a client-side navigation to another job resets it.

## 4. Verification

- [x] 4.1 `pnpm --dir web test` green (the 1006-test baseline plus the new module's), then
  `pnpm --dir web check` and the repo's eslint over the touched files, clean of new issues.
- [x] 4.2 Visually verify in headless Chrome against a locally served job page: the strip
  renders, the Company tab loads and shows facts plus About, a job with no company slug
  shows no strip, and the empty-company case reads correctly.
- [x] 4.3 Assert over CDP, before any click, that each tab's `aria-controls` resolves to a
  real element and that the inactive panel computes to `display: none`.
- [x] 4.4 Confirm the server-rendered HTML of a job page contains no company description
  or facts, and still contains the link to `/companies/<slug>`.
