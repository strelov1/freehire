# companyname

Resolves a real company **display name** for boards whose ingested `jobs.company`
is still a squished slug (e.g. `lbresearch`, `gs1ca`, `afcb`). Most ATS adapters
set `Job.Company = e.Company` straight from the board file, so a slug in that file
becomes the company's name — leaking into the UI label, the public
`/companies/<slug>` URL, and breaking logo.dev's *name* endpoint (which 404s on a
squished single token, degrading the logo to a monogram).

Consumed by `cmd/backfill-company-names`.

## Pieces

- **`SlugLike(name)`** — the authoritative "is this still a slug" test (single
  lowercase token, ≥1 letter; hyphens/digits allowed). The SQL filter is an
  approximation; this is the gate the worker trusts.
- **`Resolver` + `Registry`** — one resolver per source, keyed by source name. A
  source with no resolver is left alone, never guessed. Only *title resolvers*
  exist so far (Pinpoint, BambooHR, Lever, Ashby): fetch the careers page and pull
  the name out of `<title>` via `ExtractTitleName` (a `<lead-in> at {Name}` prefix
  or a trailing `{Name} Careers`, with a stray `| …` section trimmed off).
- **`BoardFromURL(source, url)`** — extracts the ATS board id from a representative
  job URL (host label for Pinpoint/BambooHR; first path segment for Lever/Ashby),
  matching what each resolver fetches against. Sources whose job URL is a vanity
  careers domain (e.g. Greenhouse's `a16z.com/about/jobs`) carry no board in the
  URL, so they get no resolver — a board-from-source-file lookup is a future seam.
- **`Accept(slug, candidate)`** — the gate applied before writing: decode HTML
  entities, reject junk (test/recruiter titles), and require a **confidence
  match** — the squished candidate contains the slug (or vice versa), or a
  ≥2-letter word-initial acronym lines up. Single-letter acronyms are too weak.

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
