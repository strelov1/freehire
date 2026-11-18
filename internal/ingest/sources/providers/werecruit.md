**werecruit traps** (all verified live 2026-09-06, across `idiap`, `axiom-services`,
`alcatel-submarine-networks` and `broadpeak`):

- **The whole listing is embedded server-side, and there is no pagination to walk.** A GET of
  `careers.werecruit.io/<locale>/<tenant>` is a classic server-rendered page (jQuery/Bootstrap, no
  SPA framework) whose `<script>` block assigns `window.allOffers = [...]` — the tenant's ENTIRE
  open-postings list. The bundled `offers-widget.js` confirms this is the whole dataset, not a
  first page: it sets `this.allOffers = window.allOffers` once and every filter/paginate
  operation afterward is a client-side `.slice()` over that in-memory array — no "load more"
  request exists. The extraction takes the FIRST `window.allOffers =` match on the page — a page
  carrying an earlier decoy occurrence (a comment, an unrelated inline snippet) would silently
  decode the wrong value; not observed on any tenant sampled, and the same first-match posture
  every regex-anchored embedded-token adapter in this file already takes (cornerstone, gr8people).
- **The locale segment is load-bearing, not cosmetic — the Dayforce-culture trap on a new
  platform.** `/fr/idiap` answers ZERO postings while `/en/idiap` answers four; IDIAP's site is
  configured for `en-gb` only (its own `hreflang="en-gb"`/`x-default` tags, no alternate-language
  link). Asking an unconfigured locale answers an empty listing, not an error — so the board
  keeps the locale (`<locale>/<tenant>`) rather than folding it off.
- **Unlike Dayforce, a multi-locale tenant does NOT slice its postings per locale — unioning is
  unnecessary here.** `broadpeak` (configured for both en and fr) returns the exact SAME two
  posting ids under either locale, each stating its own full `Languages` array. One valid locale
  already gives the tenant's whole catalogue; there is no per-locale subset to union like
  Dayforce's translations.
- **`Address_State` is a two-letter ISO COUNTRY code, not a US state**, despite the field name —
  confirmed `"FR"` on every French posting sampled and `"CH"` on IDIAP's, never a US abbreviation
  on a non-US address. Read through `countryFromCode`.
- **The listing carries no description at all.** Every field this adapter maps is on
  `window.allOffers`; the body lives only on the posting's own page, in a server-rendered
  `<div class="description rich-text …">` block (confirmed on an 88 KB detail page) — a plain
  exact class-token match, distinct from an unrelated `description-blocks` div further down the
  same page. Every board found is small (single digits to low tens of postings), so this is the
  Factorial shape (hydrate every posting every crawl) rather than a `HydratingSource`.
- **`TimeTranslated` ("Full time"/"Part time") is the platform's own schedule label** and maps
  cleanly to `full_time`/`part_time` on every posting sampled; `TypeTranslated` (e.g. "Temporary
  contract - 30 months") combines a contract-type enum with a duration into one free-text
  sentence with no clean split observed, so it is left unmapped.
- **No adapter-scale request volume was sent during this research** — like jobappnetwork and
  gr8people, this entry does NOT claim "no metering observed". Ship without a pacer and watch
  `board_health`.
