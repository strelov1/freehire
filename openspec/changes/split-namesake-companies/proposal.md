## Why

A company is keyed by its NAME (`normalize.CompanySlug`), and the facts that identify it —
`company_info.website`, `linkedin`, `employee_count` — are written onto whichever slug a
backfill matched by that name. Unrelated employers that share a short name therefore share
one company, and the identifying facts of one land on another:

- `motive` mixes the Greenhouse board `motive` (a creative agency, wearemotive.com) with a
  Polymer dealer-site vendor, while carrying Motive Technologies' website (gomotive.com),
  LinkedIn page and a `size_band` of 5000+ against a `company_sizes` of 51-200. Motive
  Technologies' own 195 open postings sit under `gomotive` (board company "GoMotive").
- `exa` carries exa.ai's website and tagline over 55 postings from a Brazilian InHire board
  and a French Recruitee board; exa.ai's own 73 postings sit under `exa-labs`.
- `parallel` mixed Parallel Systems (Greenhouse) with parallel.ai's Ashby board.

So `GET /companies/by-domain?domain=gomotive.com` answers `motive`, and a caller who finds
a company by name or by domain is sent to the wrong employer's jobs. An external benchmark
(Datacircle, 2026-10-28) scored these three companies' job lists at 10–20 of 100 (measured
on the replica 2026-11-24). `docs/agents/company-identity.md` has a way to MERGE two slugs
of one employer and no way to tell two employers under one slug apart.

## What Changes

- **A board names its employer's domain.** `boards` gains a nullable `company_domain`
  (a bare host). Recognition and harvest fill it where the ATS states the employer's site;
  a curator can set it. It is evidence about the board's employer, not about a name.
- **Domain evidence decides where identifying facts go.** A company-info writer that
  carries a website (Wikipedia, YC, company-info backfill, the employer's claim) SHALL NOT
  write it onto a slug whose own boards name a DIFFERENT domain, and SHALL prefer the slug
  whose boards name that domain. `by-domain` answers the slug whose boards name the domain
  before a slug whose `company_info` merely states it.
- **A namesake report.** A read-only worker lists hiring slugs fed by boards that name two
  or more different domains, or whose `company_info.website` disagrees with every board
  domain it has — the split candidates, in the same spirit as
  `cmd/report-company-website-twins` for merges.
- **A `different` correction.** Beside the existing `same` relation
  (`internal/job/companyrelation`), a moderator can approve "these boards are a different
  employer": the board is re-keyed to a new slug (the name plus its domain's leading
  label, e.g. `motive-wearemotive`), its open postings move, and the decision is recorded
  so a later crawl keeps the board on its own slug.

## Capabilities

### Modified Capabilities

- `companies`: a board's postings resolve to the company its domain names; namesakes split;
  website-bearing company-info writers respect board domain evidence.

## Impact

- `migrations/` — `boards.company_domain`, and a board-to-slug override table that, like
  `company_slug_aliases`, is not derived from `jobs` and survives a company emptying out.
- `internal/ingest/boardcatalog`, `internal/ingest/atsboard`, `cmd/harvest-*` — fill the domain.
- `internal/ingest/pipeline` — the canonical-slug resolution consults the board override
  before the name (one resolved map, as the coverage gate already requires).
- `internal/job/companyrelation`, `/moderation?tab=companies` — the `different` decision.
- Company-info writers and `ListCompaniesBySiteHost` — the domain precedence.
- Not in scope: re-keying non-ATS (aggregator) postings, which carry no board domain.
