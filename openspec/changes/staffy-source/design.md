## Context

Confirmed live against `jobs.wearestaffy.com`:

- `GET /vacantes` is fully server-rendered static HTML (no XHR, no client-side data
  fetch at all — confirmed via a full CDP network capture: zero XHR/Fetch requests fire
  after the initial document load). Every open posting is an
  `<article class="job-card"><a href="/positions/<slug>">...</a></article>`, and the
  page separately states a declared total: `<h2 class="section-kicker-title
  accent">61 activas</h2>` — verified live to equal the actual count of distinct
  `job-card` links (61 == 61).
- Each posting's own detail page (`/positions/<slug>`) is likewise static HTML. Its
  `.metadata` block is exactly three `<span>` elements in a fixed order: location
  (free text, e.g. `Argentina`, `LATAM`, `AR - UY - COL`), work arrangement (free text
  in Spanish, e.g. `Remoto`, `Hibrido - 2 veces por semana`, `Hibrido (Puerto Madero)`,
  and a `Remoto / Hibrido (...)` compound), and a seniority label. **Sampled across 30+
  of the 61 live postings** (not just the first few — an earlier draft of this design
  sampled only ~5 and would have shipped an incomplete mapping missing `Jr`, `Ssr`,
  `Staff`, and the capitalization variant `Semi Senior`, all found only once the sample
  was widened): the standard Argentina/LatAm-market IT abbreviations `Jr`/`Ssr`/`Sr` for
  Junior/Semi-Senior/Senior appear alongside their spelled-out forms, plus `Staff`, plus
  at least one genuinely ambiguous compound label (`Senior/ Semi senior`) a posting
  itself doesn't commit to one level.
- The page body below is a sequence of `<h2>`-delimited prose sections — About the
  company, About the role, Responsibilities, Requirements, Nice to have, Beneficios
  (Benefits) — none reliably present on every posting (About the company is absent on
  most sampled postings) but safe to concatenate wholesale into one description: the
  extraction code names no section explicitly, it collects every `h2`/`p`/`ul` direct
  child after the metadata block, whatever headings a given posting happens to carry.
  Most of these sections render as `<ul>`/`<li>` lists on live postings, not a single
  `<p>` — confirmed the shared `innerHTML` helper serializes nested list items correctly.
- The listing's own combined `<p class="eyebrow">Argentina · Hibrido (2 veces por
  semana)</p>` text is NOT used — the detail page's three separate spans are strictly
  cleaner to parse than un-splitting a `·`-joined free-text pair, and the detail fetch is
  already required for the description.
- "About the company" is NOT a usable per-posting company signal. Sampled across several
  postings: usually absent entirely; where present, sometimes describes STAFFY ITSELF
  ("We are a young and fast-growing recruiting company... recruitment, outsourcing, and
  team-building services") and sometimes an ANONYMIZED end-client ("It works with
  leading AI organizations and high-growth technology companies...", never naming it).
  Staffy is a recruiting/staffing agency, not the employer of record for any of its
  posted roles in the traditional sense — the same shape `recruiterflow`'s agency boards
  already established.

## Goals / Non-Goals

**Goals:**
- Crawl Staffy's whole board from one static listing fetch (enumeration, verified against
  its own declared total) plus one detail fetch per posting for description, location,
  work mode, and seniority — all read from clean, fixed DOM structure, no JSON API or
  embedded-JS-variable parsing needed at all (unlike every prior adapter in this
  initiative).

**Non-Goals:**
- No per-posting `Company` mapping — see Context; `CompanyEntry.Company` (the
  curator-configured "Staffy") is used for every posting.
- No `EmploymentType`/`Skills`/`Salary*` mapping. None of these fields appear anywhere on
  either page in any structured or even loosely-labeled form.
- No use of the listing's own combined eyebrow text — see Context.

## Decisions

- **Boardless, like `lumenalta`.** Staffy is a single recruiting agency's own domain, not
  a multi-tenant SaaS platform other companies could be onboarded under — there is no
  board id to parse from a URL, and no `internal/ingest/atsboard` entry is warranted (the
  domain names exactly one company).
- **`fullBoardListing` applies, verified against the listing's own declared total** — the
  same "prove wholeness against a declared count" posture `scalis`/`recrutei`/`pyjamahr`
  already established, here against `"61 activas"` rather than a JSON `count`/`total`
  field. A mismatch between the declared total and the actual `job-card` count fails the
  whole `Fetch`, the same as those three.
- **Seniority maps from the detail page's third metadata span**, case-insensitively:
  `Junior`/`Jr`→`junior`, `Semi senior`/`Ssr`→`middle` (the standard LatAm-market term
  and its abbreviation for mid-level), `Senior`/`Sr`→`senior`, `Staff`→`staff` — a
  confirmed vocabulary from a 30+-posting sample (unlike `pyjamahr`'s ambiguous
  `entry-level`/`associate`, which stayed unmapped for exactly this reason). A
  compound/ambiguous label (e.g. `Senior/ Semi senior`) or any other value maps to `""`
  rather than a guess.
- **Work mode maps from the detail page's second metadata span via a local, Spanish-aware
  prefix check** (`Remoto`→`remote`, `Hibrido`/`Híbrido`→`hybrid`, else `""`) rather than
  extending the shared `workplaceTypeMode` helper, which only recognizes English spellings
  — this field's values are much freer text than a clean enum (e.g. `Hibrido - 2 veces
  por semana`, `Hibrido (Puerto Madero)`), so a prefix/substring check stays local to this
  adapter rather than growing the shared helper's vocabulary for one board's dialect.
- **Location is the first metadata span verbatim** (no attempt to normalize the varied
  country/region abbreviations — `AR (Bs As)`, `CABA o AMBA`, `LATAM` — into a controlled
  geography; the dictionary downstream is better positioned to do that than a guess here).
- **A detail-fetch failure marks only that posting Unreadable, never the whole board** —
  the listing already proves the posting exists and names it, the same posture every
  other listing-then-detail adapter in this package gives.
- **A detail page whose `.metadata` block cannot be found at all is ALSO marked
  Unreadable**, not just a transport failure. Without it every structured field is lost
  and `staffyDescriptionHTML`'s direct-child walk has no anchor for where the prose
  sections begin — silently shipping an empty-location/empty-work-mode job (or a
  description missing its intended boundary) would be worse than dropping it, the same
  "page read successfully but doesn't have what we need" posture the missing-`article`
  check already gives.

## Risks / Trade-offs

- [Only one tenant, by construction — Staffy IS the tenant] → no multi-tenant risk to
  weigh; this is a single-company boardless adapter like `lumenalta`.
- [The listing's declared total and the DOM class names (`job-card`, `metadata`,
  `section-kicker-title`) are unversioned, undocumented frontend markup] → could change
  without notice, the same risk every DOM-scraping adapter in this codebase already
  accepts (`selfrecruit`, `geekhunter`).

## Migration Plan

No data migration. Ship the adapter, merge, deploy, then add the `staffy` board (boardless
— no `--board` flag) by hand via `cmd/add-board` to close `board_submissions` id 193.
