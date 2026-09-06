## Context

See proposal.md - Why. All of the following was verified live on 2026-09-06 against four
tenants — `idiap` (en-only), `axiom-services` (fr-only), `alcatel-submarine-networks` (currently
zero open postings), and `broadpeak` (both en and fr configured) — and is written up for
`internal/ingest/sources/AGENTS.md` (task 3).

**Platform shape.** `careers.werecruit.io` is a single Azure-hosted (ARRAffinity cookie), classic
server-rendered (jQuery/Bootstrap, no SPA framework) multi-tenant site. Every tenant's public URL
is `careers.werecruit.io/<locale>/<tenant>/…`, and the listing page for `<locale>/<tenant>`
embeds the tenant's OPEN postings directly in the HTML as `window.allOffers = [...]` — a plain
JSON array, no pagination, no separate API call. The bundled `offers-widget.js` confirms this is
the whole dataset: it assigns `this.allOffers = window.allOffers`, then every filter/paginate
operation in the widget is a client-side `.slice()` over that in-memory array — there is no
"load more" request to a server endpoint.

**Locale is load-bearing, not cosmetic.** `/fr/idiap` answers ZERO postings while `/en/idiap`
answers four — IDIAP's site is configured for `en-gb` only (confirmed by the page's own
`hreflang="en-gb"`/`x-default` tags and no alternate-language link). This is the Dayforce-culture
trap in a new platform: asking a site for a locale it does not publish in answers an empty list,
not an error. Unlike Dayforce, though, a tenant configured for MULTIPLE locales (`broadpeak`, en
+ fr) returns the exact SAME posting set — same ids, each posting stating its own full
`Languages` array — under either of its configured locales; there is no per-locale SLICE of the
catalogue the way Dayforce's translations are. So there is no "union the locales" question here:
one valid locale already gives everything, and an invalid one gives nothing.

**Posting shape** (listing fields actually used; per-posting custom question/answer fields and
platform bookkeeping ids are irrelevant and omitted):
```json
{
  "Id": "49895d7f-fe57-4d0d-92d7-126837be0ea3",
  "TitleTranslated": "Postdoctoral Researcher for SNF-funded StrOntEx and MetaboLinkAI projects F/M",
  "Url": "https://careers.werecruit.io/en/idiap/offers/postdoctoral-researcher-...-49895d",
  "Address_City": "Martigny", "Address_Region": "Valais", "Address_State": "CH",
  "TimeTranslated": "Full time",
  "PublicationStartDate": "2026-08-20T15:07:34.2606458+00:00"
}
```
`Address_State` is a two-letter ISO COUNTRY code despite the "State" name (confirmed: `"FR"` on
every French posting sampled, `"CH"` on IDIAP's), so it maps to `Countries` via
`countryFromCode`, not a literal US state. `TimeTranslated` is "Full time"/"Part time" on every
posting sampled — the platform's own labels for `vocab.EmploymentTypeValues`' `full_time`/
`part_time`. The listing's `Url` is already the posting's full canonical URL, so no URL
construction is needed for the detail fetch or for `Job.URL`.

**No description in the listing.** Every field above is on `window.allOffers`; the body is only
on the posting's own page, in a server-rendered `<div class="description rich-text …">` block —
confirmed on IDIAP's detail page (88 KB). Detail pages are the same order of size as
`factorial.go`'s (13-24 KB there; werecruit's ran to ~88 KB on the one sampled, still small
against a board that itself never exceeded single digits to low tens of postings on every tenant
found), so this is the Factorial shape (fetch every detail every crawl, no `HydratingSource` seen
gate), not the Workstream/Gusto one (large boards where re-hydrating every run would be
wasteful).

## Goals / Non-Goals

**Goals:**
- Crawl one werecruit board (`<locale>/<tenant>`) to the standard this catalogue holds detail-
  fetching adapters to: list once, hydrate every posting's body, map every field the platform
  states.
- Treat the locale as load-bearing board identity, so an onboarded board always resolves to the
  tenant's actual catalogue rather than silently to an empty one.

**Non-Goals:**
- Locale auto-detection or a fallback chain across a tenant's configured locales. Every apply
  link the harvest sees already carries the tenant's correct, working locale (it is the URL the
  browser rendered), so there is no board-onboarding case this would need to solve.
- Per-posting custom question/answer fields (`PostQuestionTemplateAnswers`) — tenant-specific
  application-form configuration, not catalogue data, the same posture `gr8people`'s Non-Goals
  take on its own custom fields.
- `Type`/`ContractDuration`/`DurationType` (a numeric contract-type enum plus a duration in
  months) — `TypeTranslated` combines them into one free-text sentence ("Temporary contract - 30
  months") with no clean split into freehire's structured fields observed across the sample; left
  to the description/title dictionaries rather than guessed at.

## Decisions

**Board = `<locale>/<tenant>`, a new `atsBoards` mode (`modePathPair`) rather than reusing
`modePathLocalePair`.** Dayforce's mode exists for an OPTIONAL locale prefix that gets DROPPED
because the site's postings are locale-independent once you are past it (the two segments that
remain are the whole board). werecruit is the opposite: the locale segment is REQUIRED and IS
part of the board, because it is the one thing that determines whether the tenant's site answers
anything at all. Folding it off (Dayforce's move) would produce a board string that cannot tell
`cmd/ingest` which locale to request. The new mode is simpler than the one it might look like a
variant of: it always takes exactly the first two path segments, with no locale-shaped-or-not
branching.

**Not a `HydratingSource`.** Every board found across the vendor's small public footprint (a
handful of research/industrial-services/telecom tenants, single digits to low tens of postings
each) is cheap to re-hydrate in full every crawl — the same reasoning `factorial.go` and
`cornerstone.go` apply, and the opposite of Workstream's/Gusto's "large board, cache what's
already seen" case. If a much larger tenant is found later, this is the seam to revisit.

**`Address_State` read as a country code, not a US state.** Every non-US posting sampled carries
its own country's alpha-2 code there (`"FR"`, `"CH"`), never a US state abbreviation on a non-US
address — reading it through `countryFromCode` (which already normalizes alpha-2 input) is
correct rather than a naming-collision trap to guard against.

## Risks / Trade-offs

- **[Undetermined rate limiting]** Only a handful of manual probes were sent. → Ship without a
  pacer and watch `board_health` before onboarding many boards, the same posture the
  `jobappnetwork`/`gr8people` entries take.
- **[`window.allOffers` extraction depends on the platform's own markup, not a documented API]**
  A future frontend rewrite could move this data elsewhere entirely (as gr8people's own Next.js
  migration presumably once did to something else). → The extraction locates the assignment by
  name and decodes exactly one JSON value from that point on (`json.Decoder`, not a hand-rolled
  end-of-array regex boundary), so it survives everything BUT the platform actually renaming or
  removing the variable — at which point the adapter fails loudly (no postings, or a decode
  error) rather than silently, the same failure shape a markup change gives any scraping adapter.
- **[Small vendor footprint]** Every tenant found in this research is small (single digits to
  low tens of open postings). Accepted — the issue's own ranking already weighs this platform at
  6 sampled hits, the smallest of the four onboarded from it, and closing out the issue's list
  does not require the platform to be large.
