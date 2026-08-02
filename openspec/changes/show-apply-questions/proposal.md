## Why

`apply_forms` holds the application form for every posting we can read one from —
the questions, their kinds, whether an answer is required. Nothing reads it. The
capture change shipped the store deliberately without a reader, on the grounds that
the reader's shape was a product decision it should not make.

That decision has been made. A candidate still cannot tell a one-click apply from
an evening of essay writing until they have committed to the form, and the answer
to "what will they ask me" is sitting in a table nobody queries.

## What Changes

- A new endpoint serves one posting's stored application form, shaped for display:
  `GET /jobs/:slug/apply-form`. It answers 404 when there is no stored form, which
  is the common case and always will be for the providers whose forms cannot be
  read.
- The job detail page fetches it alongside the similar-jobs and other-locations
  requests it already makes, degrading to nothing on any failure, and renders the
  questions in a block on the page.
- The display projection collapses the standard fields into one entry, drops the
  platform's equal-opportunity survey, and renders each question's control kind as
  a word.

Two deliberate reversals and exclusions, recorded because they contradict either a
previous decision or an obvious expectation:

- **The question text is shown verbatim.** The capture change decided the opposite —
  store the text, display only a derived summary — because the questions are the
  employer's writing. That is reversed here: a summary is a worse answer to "what
  will they ask me" than the questions themselves.
- **No derived facts at all.** No question count, no time estimate, no detection of
  whether salary or a visa is asked. Counting is exact but reading meaning out of
  employer prose is guessing, and a wrong guess beside the apply button would
  discredit the accurate parts of the block along with itself.
- **No provider fallback.** A posting whose platform we cannot read shows nothing
  rather than something generic about the platform.
- **Out of scope: a filter.** Filtering the catalogue by what applying costs needs a
  filterable attribute in the search index and a full reindex to register it. That
  cannot be justified before anyone has shown they read this block at all.

## Capabilities

### New Capabilities
- `apply-form-display`: serving a stored application form for one posting and
  showing its questions on the job page — the endpoint, the display projection, and
  what it deliberately omits.

### Modified Capabilities
<!-- none: the endpoint is new, and internal/jobview is deliberately untouched -->

## Impact

- `internal/applyform` — a display projection over the stored `Form`.
- `internal/handler` — one new read endpoint and its route.
- `internal/db` — one query, loading a job's form by public slug.
- `web/src/routes/jobs/[slug]/` — a fourth parallel fetch in the SSR loader.
- `web/src/lib/components/` — one new component.
- No migration, no search-index change, no change to `internal/jobview` and
  therefore none to the indexed documents.
