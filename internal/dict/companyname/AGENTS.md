# companyname

Resolves a real company **display name** for boards whose ingested `jobs.company`
is still a squished slug (e.g. `lbresearch`, `gs1ca`, `afcb`). Most ATS adapters
set `Job.Company = e.Company` straight from the board file, so a slug in that file
becomes the company's name — leaking into the UI label, the public
`/companies/<slug>` URL, and breaking logo.dev's *name* endpoint (which 404s on a
squished single token, degrading the logo to a monogram).

Consumed by `cmd/backfill-company-names`, which writes each accepted name to the
`boards` row first and to `jobs` second. The board is the half that makes it stick: the
adapter copies `boards.company` onto every posting and the job upsert overwrites
`jobs.company`, so until the worker wrote the board too, every rename it applied was
undone by that board's next crawl.

## Pieces

- **`SlugLike(name)`** — the authoritative "is this still a slug" test (single
  lowercase token, ≥1 letter; hyphens/digits allowed). The SQL filter is an
  approximation; this is the gate the worker trusts.
- **`Resolver` + `Registry`** — one resolver per source, keyed by source name. A
  source with no resolver is left alone, never guessed. Only *title resolvers*
  exist so far (Pinpoint, Lever, Ashby): fetch the careers page and pull the name
  out of `<title>`, each source with its own extractor — Pinpoint's
  `ExtractTitleName` (a `<lead-in> at {Name}` prefix or a trailing `{Name}
  Careers`, with a stray `| …` section trimmed off), Lever's bare-name title,
  Ashby's `{Name} Jobs`. BambooHR is deliberately absent: its careers page is a
  client-rendered SPA whose static `<title>` is platform boilerplate, never the
  tenant's name, so a resolver would always return `""` — `BoardFromURL` still
  handles `bamboohr` URLs, there is just no resolver wired for it.
- **`BoardFromURL(source, url)`** — extracts the ATS board id from a representative
  job URL (host label for Pinpoint/BambooHR; first path segment for Lever/Ashby),
  matching what each resolver fetches against. Sources whose job URL is a vanity
  careers domain (e.g. Greenhouse's `a16z.com/about/jobs`) carry no board in the
  URL, so they get no resolver — a board-from-source-file lookup is a future seam.
- **`Accept(slug, candidate)`** — the gate applied before writing: decode HTML
  entities, reject junk (test/recruiter titles), and require a **confidence
  match** — the squished candidate contains the slug (or vice versa), or a
  ≥2-letter word-initial acronym lines up. Single-letter acronyms are too weak.

## Placeholder boards

A second, separate case: a board whose `company` is not a slug but **the board id itself** —
`fa-etjg-saasfaprod1.fa.ocs.oraclecloud.com/Maggianos`, a Paylocity GUID — written verbatim by
a harvest that never resolved the tenant's name. Measured 2026-10-23: ~2,200 companies carrying
~156k open postings under a host path, ~7,400 carrying ~44k under a GUID.

- **`NewPlaceholderRegistry`** — resolvers keyed by provider that take the board id and read
  the name the ATS states for exactly that board: Paylocity's listing title
  (`{Name} - Job Opportunities`), UKG Ready's `format-settings` `company.name`, Oracle's site
  `og:site_name` through `ExtractOracleSiteName` (drops a lead-in, cuts at a careers or recruiting marker, refuses a site named for
  its purpose — "Student Casual"). Taleo, ADP and Workday were tried and are absent on purpose;
  the constructor's comment says why for each.
- **`AcceptPlaceholder`** — no confidence match: the lookup is by the exact board id, so there
  is no slug to compare against and nothing an unrelated page could slip through (join's
  argument in `Accept`). It refuses only what is no improvement — empty, junk, the placeholder.

`cmd/backfill-company-names` runs this as its second pass, driven by the BOARD
(`ListPlaceholderCompanyBoards`), and renames the board and its stored jobs in one transaction:
the pass finds its work by the board, so a board renamed without its jobs is never listed again.

## Design stance

Conservative on purpose: a wrong-but-plausible name reads worse than the monogram
fallback, so a candidate is applied only when it demonstrably shares text with the
slug. That means genuine **rebrands** whose new name shares nothing with the slug
(e.g. `lbresearch` → "Centellic") are deliberately *not* auto-resolved — they need
a human or a different signal (the domain). Resolvers return `""` (not an error)
when a source yields no usable name; errors are reserved for transport failures so
one dead board never aborts a run.

The confidence rules and the accept/reject corpus are the same ones validated by
the manual Pinpoint pass (PR #825): 104 accepted, 10 correctly rejected across 241
slug boards.
