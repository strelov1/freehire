## Context

A CV is rendered on demand (`GET /me/cvs/:id/pdf`, Typst, nothing persisted), downloaded, and
uploaded by the candidate into whatever ATS the employer runs. From that point the product
sees nothing until a reply arrives in the connected mailbox — or does not.

The prior art is the Tracer Links feature of DaKheera47/job-ops
(`orchestrator/src/server/services/tracer-links.ts`, `db/schema.ts`). Its shape is sound and is
followed here. It was built for a single-user self-hosted instance, and three of its decisions
do not survive the move to a multi-tenant product with agents; each is called out below.

The full exploration behind these decisions, including the options rejected before this
change was proposed, is in `docs/superpowers/specs/2026-07-31-cv-tracer-links-design.md`.

Current constraints that shape the work:

- `web/nginx.conf` routes only `/api/` and `/health` to the Go backend; everything else goes to
  SvelteKit.
- Only `classic-ats.typ` emits `link()`. The other five templates print links as inert text.
- `ListUserJobs` already carries four correlated subqueries per row and is server-rendered.
- `cvs.job_id` exists and is nullable, so a CV can already be tied to a job.
- `cvs.data` is the jsonb the candidate edits and the tailoring agent patches.

## Goals / Non-Goals

**Goals:**

- Tell a candidate whether a CV they sent was opened, with per-link detail and a marker on the
  application it belongs to.
- Keep the feature inert for anyone who does not deliberately switch it on, per CV.
- Collect the minimum about the person clicking that still answers "how many people".

**Non-Goals:**

- A hosted public CV page. Rejected in favour of PDF coverage: every ATS takes a file, few take
  a link.
- A global analytics page or per-day charts. At single-digit click counts a time series is
  noise.
- A cross-user per-company signal ("does this employer open CVs"). The model permits it later
  through `cvs.job_id`; nothing is built for it now.
- Any change to how application silence is derived.

## Decisions

### The redirect is served by Go, not SvelteKit

`app.Get("/cv/:token")` beside `/health`, plus a `location /cv/` in `web/nginx.conf`.

*Alternative:* a SvelteKit `+server.ts` that calls the API, avoiding the nginx edit. Rejected:
the client IP would then reach the API in a header set by the SSR layer, and anything able to
reach the API directly could forge it. That forged value feeds the distinct-visitor count. One
line of nginx config is cheaper than an input we cannot verify.

### Rewriting happens in the render payload, never in the document

`renderPayload` gains a `link_hrefs` array aligned by index with `links`, and the same for
projects. Templates move from `link(l)[#l]` to `link(href)[#l]`, falling back to `l` when
`href` is empty.

*Alternative:* rewrite `cv.Document` before rendering. Rejected: that is the persisted shape the
candidate edits and the agent patches; putting tracking state in it means every consumer of a
CV document has to know about tracing.

### Visible text stays the candidate's, only the target changes

*Alternative:* replace both, as job-ops does — which also traces a reader who copies the text
rather than clicking, and works in templates without clickable links. Rejected: it puts an
opaque product URL where a recruiter expects `github.com/name`.

The cost is accepted knowingly: text-not-matching-target is the pattern anti-phishing filters
look for, and it makes clickable links a precondition rather than a nicety.

### All six templates must emit clickable links first

Not adjacent cleanup — with native visible text, an unclickable link is untrackable, so five of
six templates would report zero. A registry-driven test covers every template, so a sixth added
later by copying `portrait.typ` cannot silently reintroduce the hole. That test only guards the
rule if CI can run it, so CI installs the same pinned typst as the prod image.

Making a link clickable says nothing about how it looks. Only `classic-ats` colours links
(`#show link: set text(fill: ...)`); the other five deliberately keep them visually identical to
body text, which is why the gallery previews changed by zero pixels. The affordance stays each
template's own design choice — copying that show rule into all six would move every preview and
alter five designs to no purpose.

The link's *target* is a separate matter from whether it is clickable, and it is not fixed by
this decision: links are stored scheme-less, so the emitted URI is relative and unfollowable in
every template today, `classic-ats` included and long predating this change. That is normalised
in the render payload (task 5.1b), not in six copies of Typst string handling.

### `last_click_at` is denormalised onto `cvs`

The click write stamps it in the same transaction, for countable clicks only. The board then
reads one column instead of a fifth correlated subquery joining three tables.

*Alternative A:* read the event table from the listing. Rejected: `ListUserJobs` is
server-rendered and already carries four such subqueries.
*Alternative B:* count from nginx access logs, by analogy with `internal/viewlog`. Rejected: the
analogy breaks. A job view needs no database to be served, so moving its counting out of the
request path is free; a token redirect must read the database to know where to send the
visitor, so the log route saves exactly one INSERT and costs a worker, a flock, a lag before
numbers appear, and a second log parser.

### Departure from prior art 1 — one `visitor_hash`, not `ip_hash` plus a fingerprint

job-ops stores both. `ip_hash` answers nothing the fingerprint does not; it is personal data
held with no use, which is pure downside on a breach. We keep `HMAC(salt, ip + user_agent)`.

Salted rather than a bare `sha256(ip)`: IPv4 has 4.3 billion addresses, so an unsalted hash of
an IP is reversible by exhaustive search on a laptop and would be anonymisation in appearance
only.

### Departure from prior art 2 — five random characters in the token, not two letters

`<company-slug>-<rrrrr>`, prefix from `cvs.job_id → jobs.company_slug` or `cv` when the CV is
tied to no job.

Two letters give 676 tokens per prefix. That is ample for one self-hosted user and wrong here:
hundreds of candidates apply to the same company and share a prefix, so by the birthday bound
collisions begin around the thirtieth token and the space is exhausted in the low hundreds.
Five characters give roughly 60 million per prefix.

### Departure from prior art 3 — the toggle is unreachable by the agent

`tracer_links_enabled` is written only by `PUT /me/cvs/:id` (cookie-only) and is not a member of
`PatchOps`, the tailoring agent's path. This mirrors the existing rule that keeps the style
block out of `PatchOps` — "the tailoring agent edits content, the candidate edits
presentation" — and the stake here is larger: consent to track a third party is the
candidate's to give.

job-ops has no equivalent problem because it has no agent writing to CVs.

### The owner's own clicks do not count

The redirect is same-origin, so the owner's session cookie rides along with a click they make
from their own downloaded PDF. Such clicks are recorded, marked, and excluded from every
presented count.

Without this, the first thing a candidate does after enabling the feature — download the PDF
and click the link to check it works — reports itself back as "your CV was opened".

### Minting is idempotent, keyed by (cv, source_path, destination_hash)

The PDF is not stored and is re-rendered on every download, so "tokens are issued at
generation" means "on every download". Without idempotency three downloads would produce three
tokens for one link and scatter the counts.

`ON CONFLICT ... DO UPDATE SET cv_id = EXCLUDED.cv_id RETURNING token` — the no-op update
exists because `DO NOTHING` returns no row, which would force a second read and reintroduce the
race the upsert removes. Same idiom as `UpsertJob`.

`source_path` is in the key on purpose: the same URL in the header and on a project gets two
tokens, because those are different events.

### Recording is best-effort; the redirect is the contract

A failed click write must not fail the redirect. This follows `RecordView`, which swallows
failures so a view cannot break a page; the stake is higher here, because a broken redirect
lives in a PDF the candidate can neither see nor fix.

### The bot flag is frozen at write time

Computed once, when the click is recorded, and never recomputed. Were it evaluated on read,
editing the pattern list would silently rewrite history — yesterday's twelve clicks reading as
nine today with no new rows. A non-`GET` method is always flagged.

### Retention lives in `cmd/prune`

180 days, swept by the repository's single hard-delete path. A second worker for one `DELETE`
is not warranted.

## Risks / Trade-offs

- **Mail-security scanners fetch links with ordinary browser user agents** → not detectable by
  user-agent rules, so "your CV was opened" is systematically overstated. Mitigated only by
  wording: the surfaces present clicks as evidence, never as proof. This is a limitation of the
  measurement, not a bug to be tuned away.
- **A redirect inside a PDF is a phishing pattern** and gateways may flag the domain. job-ops
  pushes this risk onto whoever runs the instance; our operator is us, and `freehire.me`
  carries all other traffic → the endpoint is token-only and never reads a destination from the
  request, so it cannot serve as an open redirect. Residual risk remains and is accepted.
- **Visible text that does not match the target is itself a phishing signal** → accepted
  deliberately in exchange for a CV that looks normal.
- **`ON DELETE CASCADE` kills links in already-sent PDFs** → accepted: erasing one's own data
  outranks a stranger's convenience, and someone deleting a CV usually wants exactly this. The
  response is an explanatory `410`, not a bare 404.
- **`ALTER TABLE cvs` takes ACCESS EXCLUSIVE** and this project has already had DDL queue
  behind a long reader and take the site down → do not run it in the 03–07 UTC `pg_dump`
  window.
- **`TRACER_LINK_SALT` missing in an environment** → the toggle cannot be enabled; existing
  tokens keep redirecting and record an empty visitor hash, and the surfaces drop the
  distinct-visitor count rather than report a wrong one.

## Migration Plan

1. Migration `0060_cv_tracer_links.sql` — two columns on `cvs`, two new tables. Applied before
   any code that reads them, outside the nightly dump window.
2. `location /cv/ { proxy_pass $backend; }` in `web/nginx.conf`.
3. Code. The feature is inert until a candidate enables it on a CV, so there is no rollout
   sequencing beyond the above.

Rollback: revert the code and the nginx location. The tables can stay — with no code minting
tokens, no link is traced, and existing tokens simply stop resolving (`410`). Dropping them is
a separate decision and loses history irreversibly.

## Open Questions

- Retention is set at 180 days by judgement, not measurement. Nothing yet says whether a
  candidate ever looks at a click older than a month.
- The per-company aggregate ("this employer opens the CVs it receives") is the strongest thing
  this data could support and the reason the model keeps `cvs.job_id` reachable — but it needs
  a sample that does not exist yet, and publishing it raises questions this change does not
  answer.
