# CV tracer links

Rewrite the outbound links in a rendered CV PDF to `freehire.me/cv/<token>`, redirect to the
real destination, and count the clicks. Opt-in per CV, off by default.

Prior art: the Tracer Links feature in [DaKheera47/job-ops](https://github.com/DaKheera47/job-ops)
(`orchestrator/src/server/services/tracer-links.ts`, `db/schema.ts`). This design follows its
shape and departs from it in three places, each noted below.

## Why

A CV leaves the product and goes dark. The candidate learns nothing between sending it and
either a reply or silence. A traced link answers one narrow question — *was this CV opened
at all* — which is the difference between "they read it and passed" and "it never reached a
human".

The answer is indirect: a click on the GitHub link inside a CV is evidence someone opened the
CV, not proof, and mail-security scanners inflate it. The UI must not overclaim.

## Decisions taken before design

| Question | Decision |
|---|---|
| What we track | Clicks on links inside the PDF, not a hosted CV page. A PDF attachment is accepted by every ATS; a link is accepted by few. |
| Opt-in level | Per CV (`cvs.tracer_links_enabled`, default false). A CV sent to a government body or a university simply never has it on. |
| Per-click data | Salted visitor hash, so "3 different people opened it" is answerable. |
| Visible link text | Stays the candidate's own (`github.com/jrivera`); only the href is traced. |
| Where the numbers show | Panel in the CV editor, plus a marker on the tracking board card. |

## Non-goals

- No hosted/public CV page. Explicitly rejected: coverage matters more than signal directness.
- No global analytics page, no per-day charts. At single-digit click counts a time series is
  noise wearing a chart's clothes.
- No cross-user aggregate (per-company "does this employer open CVs"). The data model permits
  it later via `cvs.job_id`; nothing is built for it now.

## Architecture

### `internal/tracerlink` — new domain package

Fiber-free and pgx-free, following `internal/jobtracking`. It owns:

- `Targets(doc cv.Document) []Target` — which links are eligible and their `source_path`.
- `Token(companySlug string) string` — token minting.
- `Classify(method, userAgent string) Client` — bot flag, device type, OS family, UA family.
- `VisitorHash(salt, ip, userAgent string) string` — HMAC.

All pure. The HTTP handler and the sqlc queries live where they always do
(`internal/handler/cv.go`, `internal/db/queries/tracer_links.sql`).

### The redirect lives in Go, not SvelteKit

`app.Get("/cv/:token")` beside `/health` (`internal/handler/handler.go:349`), plus
`location /cv/ { proxy_pass $backend; }` in `web/nginx.conf`, which today routes only `/api/`
and `/health` to the backend.

The alternative — a SvelteKit `+server.ts` calling the API — avoids the nginx edit but makes
the client IP arrive at the API in a header set by the SSR layer. Anything that can reach the
API directly could then forge it, and the forged value feeds the unique-visitor count. One
line of nginx config is cheaper than an unverifiable input.

### Rewriting happens in the render payload, not in the document

`cvs.data` is the jsonb the candidate edits and the tailoring agent patches. Tracer state does
not belong in it.

`renderPayload` (`internal/cv/renderer.go:130`) gains `link_hrefs`, an array aligned by index
with `links`, and the same for projects. Templates move from `link(l)[#l]` to `link(href)[#l]`,
falling back to `l` when `href` is empty. A stored CV is unchanged, and a CV rendered with the
toggle off produces visually identical output to today (verified as SVG — see Testing).

**Five of six templates must be fixed first.** Only `classic-ats.typ:53` emits `link()`; in
`portrait`, `sidebar`, `centered`, `modern-sans` and `headshot` a link is plain text that
cannot be clicked. With the visible text staying native, an unclickable link is untrackable —
so this is a precondition, not cleanup.

## Data model

Migration `0060_cv_tracer_links.sql`:

```sql
ALTER TABLE cvs ADD COLUMN tracer_links_enabled boolean NOT NULL DEFAULT false;
ALTER TABLE cvs ADD COLUMN last_click_at timestamptz;

CREATE TABLE cv_tracer_links (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cv_id            uuid NOT NULL REFERENCES cvs(id) ON DELETE CASCADE,
    token            text NOT NULL UNIQUE,
    source_path      text NOT NULL,
    destination_url  text NOT NULL,
    destination_hash text NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (cv_id, source_path, destination_hash)
);

CREATE TABLE cv_link_clicks (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tracer_link_id uuid NOT NULL REFERENCES cv_tracer_links(id) ON DELETE CASCADE,
    clicked_at     timestamptz NOT NULL DEFAULT now(),
    is_likely_bot  boolean NOT NULL DEFAULT false,
    is_owner       boolean NOT NULL DEFAULT false,
    device_type    text NOT NULL DEFAULT 'unknown',
    os_family      text NOT NULL DEFAULT 'unknown',
    ua_family      text NOT NULL DEFAULT 'unknown',
    referrer_host  text NOT NULL DEFAULT '',
    visitor_hash   text NOT NULL DEFAULT ''
);

CREATE INDEX cv_link_clicks_link_idx ON cv_link_clicks (tracer_link_id, clicked_at DESC);
CREATE INDEX cv_tracer_links_cv_idx  ON cv_tracer_links (cv_id);
```

`source_path` is the location in the document (`header.links[1]`, `projects[0].link`), and it
is part of the uniqueness key on purpose: the same GitHub URL in the header and on a project
gets two tokens, because "clicked the header link" and "clicked through to that project" are
different events and merging them would erase the only interesting distinction.

`destination_hash` (sha256 hex) rather than the URL itself keeps the unique index key at a
fixed 64 bytes; destinations are arbitrary-length strings with query parameters.

### Departure 1 — one `visitor_hash`, not `ip_hash` + `unique_fingerprint_hash`

job-ops stores both. `ip_hash` answers no question that the fingerprint does not, so it is
personal data held with no use — pure downside on a breach. We keep
`HMAC(salt, ip + user_agent)` and nothing else.

The HMAC is salted rather than a bare `sha256(ip)` because IPv4 has 4.3 billion addresses: an
unsalted hash of an IP is reversible by exhaustive search on a laptop, and would be
anonymisation in appearance only. The salt comes from `TRACER_LINK_SALT`.

### `last_click_at` on `cvs` — the denormalisation

`ListUserJobs` (`internal/db/queries/user_jobs.sql:127`) already carries four correlated
subqueries per row (`email_count`, `reminder_fire_at`, `last_activity_at`,
`has_pending_suggestion`) and is server-rendered. Reading the click history there would mean a
fifth, joining three tables (`cv_link_clicks → cv_tracer_links → cvs`), since only `cvs.job_id`
connects a click to an application.

Instead the click write also stamps `cvs.last_click_at`, in the same transaction, for
non-bot clicks only. The board then reads one column via `(user_id, job_id)`. The detail panel
in the CV editor reads the event table directly — it renders one CV, not a hundred cards.

The rejected alternative was a rollup from nginx access logs, by analogy with
`internal/viewlog`. The analogy breaks: a job view needs no database to be served, so moving
its counting out of the request path is free. A token redirect *must* read the database to
know where to send the visitor, so the log route saves exactly one INSERT and costs a worker,
a flock, a lag before numbers appear, and a second log parser.

### Deletion and its cost

`ON DELETE CASCADE` on `cv_id` is the right to be forgotten, implemented in the schema: delete
the CV and both the tokens and every click on them go with it.

It has a real cost, accepted knowingly: links in already-sent PDFs die. A recruiter opening a
year-old CV gets a dead link where GitHub was. The candidate's ability to erase their own data
outranks a stranger's convenience, and someone deleting a CV usually wants exactly this — but
the response must be an explanatory 410 page ("this link is no longer active"), not a bare 404.

## Flows

### Minting, at render time

`GET /me/cvs/:id/pdf` renders Typst on every request; nothing is stored. So "tokens are issued
when the PDF is generated" means "on every download", and idempotency is a precondition for the
feature working at all — otherwise three downloads produce three tokens for one GitHub link and
the counts scatter across them.

```sql
-- name: UpsertTracerLink :one
INSERT INTO cv_tracer_links (cv_id, token, source_path, destination_url, destination_hash)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (cv_id, source_path, destination_hash) DO UPDATE SET cv_id = EXCLUDED.cv_id
RETURNING token;
```

The no-op `DO UPDATE` exists because `ON CONFLICT DO NOTHING` returns no row: on conflict
`RETURNING` yields nothing and the caller would have to read the token in a second statement —
reintroducing the race the upsert removes. Same idiom as `UpsertJob`.

Eligibility: links are stored without a scheme (`"github.com/jrivera"`, `internal/cv/preview.go:20`),
so a target qualifies when it normalises to http(s). `mailto:`, `tel:`, empty values, and our
own domain are skipped.

Token format is `<prefix>-<rrrrr>`, where `prefix` is the company slug reached through
`cvs.job_id → jobs.company_slug` (or `cv` when the CV is tied to no job) and `rrrrr` is five
random lowercase alphanumerics. Uniqueness is enforced by `cv_tracer_links.token UNIQUE`, with
a retry on collision.

job-ops uses two letters here. That is 676 tokens per prefix, which is ample for a single-user
self-hosted instance and wrong for us: hundreds of candidates apply to the same company, all
sharing one prefix, and by the birthday bound collisions start around the thirtieth token and
the space is exhausted in the low hundreds. Five characters give ~60 million per prefix.

The recruiter sees the token on hover and in the address bar during the redirect, and a
readable company name there is less alarming than an opaque string — it is their own company.

### Redirect

`GET /cv/:token` → look up destination → record click → `302`.

The click write is best-effort: if the INSERT fails the redirect still happens. This follows
the existing convention for `RecordView` ("failures are swallowed and must not break the
page"), and the stake is higher here — a broken redirect lives in a PDF the candidate can
neither see nor fix.

Unknown or deleted token → 410 with an explanatory page.

### Bot handling

The UA regex is taken from job-ops. Added: **a `HEAD` request is always a bot** — a human in a
browser sends `GET`.

`is_likely_bot` is written once, at click time, and never recomputed. Were it evaluated at read
time, editing the regex would silently rewrite history: yesterday's 12 clicks would read as 9
today with no new rows.

This does not solve the real problem. Corporate mail-security scanners fetch links with
ordinary browser user agents and are not detectable this way, so "a recruiter opened your CV"
is systematically overstated. The UI must show clicks as evidence, never as proof.

### Retention

Clicks older than 180 days are deleted by `cmd/prune`, the repository's single hard-delete
path. A second worker for one `DELETE` is not warranted.

### The toggle is not reachable by an agent

`tracer_links_enabled` is written only by `PUT /me/cvs/:id` (`mw.cookie`) and is **not** a
member of `PatchOps`, so `PATCH /me/cvs/:id` (`mw.key`, the tailoring agent's path) cannot set
it. This mirrors the existing rule for the style block — "the tailoring agent edits content,
the candidate edits presentation" — and here the stake is larger: consent to track a third
party is the candidate's to give, and an agent must not be able to give it on their behalf.

## Configuration

`TRACER_LINK_SALT` — required to enable the toggle. Absent, `PUT /me/cvs/:id` rejects turning
`tracer_links_enabled` on, while already-issued links keep redirecting and record clicks with
an empty `visitor_hash`. Degradation is explicit: the panel then reports clicks without the
distinct-visitor count rather than quietly reporting wrong numbers.

## Surfaces

**CV editor panel** (`/my/cvs/[id]`): the toggle, and per link — clicks, distinct visitors,
last click, with a "include likely bots" switch. One `GET /me/cvs/:id/tracer-links`.

**Tracking board** (`/my/tracking`): a "CV opened 2d ago" marker beside the existing silence
badge, from `cvs.last_click_at` via `cvs.job_id`. Two readings at once, as with the
`silent` + `chased` pair the board already shows: they have not answered in 24 days, and
someone opened the CV yesterday.

**Privacy policy** must gain a paragraph: what a traced link records, that it is opt-in per CV
and off by default, and the 180-day retention.

## Testing

Unit (`internal/tracerlink`): eligibility (scheme-less, `mailto:`, `tel:`, own domain,
malformed), token format, UA classification including the `HEAD` rule, hash stability under a
fixed salt.

Integration (`internal/db`, build tag): rendering the same CV twice yields the same tokens;
changing a destination yields a new token while the old one still resolves.

Handler: unknown token → 410; a failing click write still returns 302; the toggle cannot be
enabled without a salt.

Two tripwires:

- **Every registered template emits a clickable link.** The test iterates the `templates`
  registry, so a sixth template added later by copying `portrait.typ` cannot silently return
  tracking to zero — which is precisely today's state in five of six. Same discipline as the
  preview generator, which iterates the registry "so the set can't drift".
- **A CV with the toggle off renders identically to today.** Guards the payload change from
  leaking into the default path.

Compare rendered output as SVG, not PDF: a Typst PDF embeds a creation timestamp, so two
renders of one source differ byte-for-byte (`internal/cv/AGENTS.md`).

## Rollout

1. Migration `0060`, applied before the code that reads the columns. `ALTER TABLE cvs ADD
   COLUMN` takes ACCESS EXCLUSIVE; this project has already had DDL queue behind a long reader
   and take the site down, so not during the 03–07 UTC `pg_dump` window.
2. `location /cv/` in `web/nginx.conf`.
3. Code. The feature is inert until a candidate turns the toggle on.

## Risks

- **Redirect domains get flagged.** A redirect in a PDF is a phishing pattern; mail gateways
  may flag `freehire.me`. job-ops pushes this risk onto whoever runs the instance — here the
  operator is us, and the domain carries all our other traffic. Mitigation: token-only routing
  with no URL ever accepted in a query parameter, so the endpoint can never act as an open
  redirect.
- **Native visible text with a traced href is itself the mismatch anti-phishing filters look
  for.** Accepted deliberately in exchange for a CV that looks normal.
- **Scanner inflation**, as above — a wording problem, not a tunable one.
