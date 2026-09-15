## Context

Confirmed live against `jobs.recrutei.com.br/digisystem` via headless-browser network
capture (Chrome DevTools Protocol, since static HTML/JS inspection found nothing — every
plausible guessed REST path returned the platform's own SPA-shell `index.html`, `text/html`,
not JSON, and only looked like a hit by status code alone):

- The frontend's actual listing call is `POST /api/v2/vacancies/per-departments/<board>`
  with a JSON body `{"search":""}` (no query params, no auth headers — a bare
  `Content-Type: application/json` POST from `curl` reproduces it exactly, confirmed).
  It returns `{"data":{"total":N,"departments":M,"vacancies":[{"department":"...",
  "total":n,"items":[{"id":108736,"title":"...","regime":"CLT","company_name":"Digisystem",
  "location":["Brasília","DF","Brasil"],"public_link":"https://jobs.recrutei.com.br/
  digisystem/vacancy/108736-...","slug":"...","client":null,...}]},...]}}` — verified that
  `data.total` equals the sum of every department's `items` length (198 = 122+3+5+68 for
  `digisystem`), and that a nonexistent tenant answers a clean HTTP 404.
- A posting's own detail page (the item's `public_link`) embeds a standard schema.org
  `application/ld+json` `JobPosting` block: `title`, `description` (rich HTML),
  `datePosted` (a Brazilian `"DD/MM/YYYY HH:MM:SS"` string, confirmed to vary per posting —
  e.g. `"14/07/2025 14:37:27"`), `employmentType`, `hiringOrganization.name`, `jobLocation`.
- **`employmentType` in the ld+json block is unreliable**: verified on two live postings
  whose LISTING `regime` differed (`"CLT"` vs `"Pessoa Jurídica"`, i.e. standard-employee
  vs. contractor), the ld+json block reported `"FULL_TIME"` for BOTH — it reads as a
  platform-side constant/default rather than a real per-posting signal, unlike every other
  ld+json adapter in this package that trusts the field.
- **`jobLocation.address.addressLocality` in the ld+json block carries a literal string
  `"undefined"`** for postings whose listing `location` array has only 1-2 elements
  (confirmed on the exact posting the `board_submissions` row named, id 157190,
  `location: ["Brasil"]`) — a frontend-templating leak, not a real value. The LISTING's own
  `location` array never showed this across all 198 sampled postings.
- The listing's `regime` field is a small, closed Brazilian labor-contract vocabulary,
  enumerated across the one sampled tenant: `CLT` (180), `Pessoa Jurídica` (5, i.e. PJ —
  legal-entity contractor), `CLT ou PJ` (1, either — genuinely ambiguous), `Não informado`
  (12, "not informed").
- No item in the sampled tenant carried a non-null `client` field — the agency/hub shape
  RecruiterFlow's `hiringOrganization` turned out to have was NOT observed here, so
  `company_name` is trusted as the actual employer.

## Goals / Non-Goals

**Goals:**
- Crawl a Recrutei tenant's open postings via one listing POST (enumeration + company name
  + location + employment type, all from the listing's own fields) plus one detail fetch
  per posting (description + post date only, via the shared `ldJobPosting` decoder).

**Non-Goals:**
- No use of the detail page's `employmentType` or `jobLocation` fields — both are unreliable
  per the Context above (a platform-side constant and a template-leak placeholder,
  respectively). The listing's own `regime` and `location` fields are used instead.
- No `SalaryMin`/`SalaryMax`/`SalaryPeriod` mapping. The listing's `salary` field is
  freeform Portuguese text ("A combinar" — "to be negotiated" — on every sampled posting),
  not a structured number; this codebase's convention is to report bounds with their unit
  or nothing at all, never parse a number out of free text.
- No `Skills` mapping. Neither the listing item nor the ld+json block carries any skills
  field at all (the ld+json's own `skills` key was present but always an empty string on
  every sampled posting) — there is nothing to map, structured or otherwise.
- No remote/work-mode structured signal. Neither payload carries a boolean or enum for it;
  `Remote` is set purely from the text heuristic (`isRemote(title + location)`), and
  `WorkMode` is left unset for the description/location parser to resolve — the same
  "never invent a structured signal" bar `humanbit-source`'s work-mode Non-Goal already
  applied.
- No tenant-discovery/harvest prober — only one live tenant (`digisystem`) is known today,
  the same reasoning `selfrecruit-source`/`scalis-source`/`humanbit-source` already gave.
- No handling for a hypothetical agency/hub shape (a populated `client` field naming the
  real end-employer). Not observed on the one sampled tenant; revisit if a future tenant
  shows it, rather than building for it now.

## Decisions

- **`fullBoardListing` applies, with an explicit completeness check.** Since the endpoint
  carries no pagination parameters or `hasMore`/`next` signal, the adapter verifies
  `data.total == sum(department item counts)` on every fetch and fails the whole `Fetch`
  loudly if they disagree — the same "prove completeness or fail hard, never ship a silent
  partial" posture `scalis-source`'s page-ceiling check and `teamtailor.ttMaxPages`
  established, applied here to a single-request listing instead of a paginated one.
- **Employment type is derived from the LISTING's `regime` field, not the detail page's
  ld+json `employmentType`** (see Context: the ld+json value was observed constant across
  differing `regime`s). `CLT` → `"full_time"` (the standard Brazilian full-time employment
  contract), `Pessoa Jurídica` → `"contract"` (a legal-entity/freelance contract structure),
  `CLT ou PJ` and `Não informado` → `""` (genuinely ambiguous or unstated — not guessed).
- **Location is taken from the LISTING's `location` array, not the detail page's
  `jobLocation`** (see Context: the detail page's address leaks the literal string
  `"undefined"` for short location arrays; the listing's own array never does).
- **Description and post date are the ONLY fields read from the detail page**, via the
  shared `ldJobPosting` decoder (`internal/ingest/sources/jsonld.go`) rather than a
  bespoke parser — the same decoder `geekhunter` already uses for its own ld+json detail
  page, so this adapter adds no new parsing machinery, only a struct selecting the two
  fields it needs.
- **A detail-fetch failure marks only that posting Unreadable, never the whole board** — the
  listing already proves the posting exists and names it; losing only its description/date
  to a transient failure is the same posture every other listing-then-detail adapter in
  this package gives (`humanbit`, `successfactors`, `geekhunter`).

## Risks / Trade-offs

- [The listing endpoint carries no documented size limit or pagination] → the one sampled
  tenant (198 postings) returned everything in a single response with no truncation
  signal; a much larger tenant might behave differently, but nothing found live suggests a
  cap, and the `data.total == sum(items)` completeness check (see Decisions) will fail
  loudly rather than silently truncate if one is ever hit.
- [The listing POST requires no auth but is undocumented — it is the SPA's own internal
  API, not a published public API] → the same posture this codebase already accepts for
  several other adapters that consume an ATS's internal frontend API rather than a
  published one (e.g. `scalis`'s RSC flight, `humanbit`'s RSC flight); it could change
  without notice, same as those.

## Migration Plan

No data migration. Ship the adapter, merge, deploy, then add `recrutei/digisystem` by hand
via `cmd/add-board` to close `board_submissions` id 115.
