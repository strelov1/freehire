## Why

The catalogue already knows which postings people actually opened yesterday, and
that knowledge never leaves the database. A daily "what the market looked at"
post is the cheapest recurring reason for someone to come back to the site, and
it costs one query plus two HTTP calls — but only if the number behind it is
honest. `job_daily_views.uniques` is not: it fuses bot-filtered page opens with
**un**filtered API reads, and bots are the majority of this host's traffic. A
public "most popular" list ranked on that number is one crawler away from being
wrong in public, every day, on our own company page.

## What Changes

- **Split the view counter into two.** `job_daily_views` gains a `page_uniques`
  column carrying only the bot-filtered page-open signal. `uniques` keeps its
  present meaning and its present value — `GET /api/v1/stats/catalog` reads it,
  and the catalogue-scale snapshot must not move because of this change.
- **`internal/application/viewlog` stops collapsing its two signals.**
  `Classify` already distinguishes `KindPage` from `KindAPI`; `Aggregate` throws
  the distinction away. It will return both counts per `(day, slug)`.
- **`cmd/rollup-views` writes both columns** in the same additive upsert.
- **New capability: a daily social digest.** A new `internal/engage/socialdigest`
  package selects the day's top postings by `page_uniques` under explicit
  editorial rules (open only, no duplicate markers, no ghost signal, a floor on
  views, at most two per company, a quarantine so a posting cannot reappear for
  seven days), records what it published, and hands the result to one or more
  publishers.
- **Publishers behind one interface, and one implementation of it.** A Discord
  incoming webhook. The channel is disabled — silently, as the rest of this
  worker fleet does — when its credentials are absent, and the dispatcher is
  written for several: one publisher failing does not stop another, and the run
  still exits non-zero.

  **LinkedIn is deliberately not in this change.** Posting to a company page
  needs LinkedIn's Community Management API, whose access request is submitted
  and awaiting LinkedIn's review — a wait measured in weeks, with no promised
  date. Until it clears there is no organization URN and no token, so a
  publisher could be neither configured nor verified against anything real. The
  seam is what this change delivers instead; adding the channel once the
  credentials exist is one file and a config branch.
- **New worker `cmd/social-digest`**, with `-dry-run` (renders the post, sends
  nothing) and `-day` (replay a specific day), plus its systemd unit and timer.
- **No historical backfill.** `page_uniques` starts at zero for every existing
  row and is correct from the first `rollup-views` run after deploy. The digest
  only ever reads the most recent day, so it is correct from day one; the old
  rows simply never gain the split, and nothing reads them for it.

## Capabilities

### New Capabilities

- `social-daily-digest`: what makes a posting eligible for the daily digest, how
  the day is chosen, the editorial rules that shape the list, the publish-once
  guarantee, and the per-channel failure behaviour.

### Modified Capabilities

- `view-count-aggregation`: the rollup requirement changes — the worker now
  materializes the page-open signal separately from the combined count, so a
  consumer can rank on a bot-filtered number. The counted signals, the dedup
  rule, the cursor and the backfill are unchanged.

## Impact

**Schema** — one migration: `job_daily_views.page_uniques` (NOT NULL DEFAULT 0)
and a new `social_digest_posts` ledger.

**Go** — `internal/application/viewlog` (`Aggregate` signature and its callers),
`cmd/rollup-views`, new `internal/engage/socialdigest`, new `cmd/social-digest`.
`internal/platform/db` is regenerated (`make sqlc`).

**Architecture** — `internal/engage/socialdigest` must be added to the block
table in `internal/platform/arch/layering/blocks.go`, or both layering guards
fail. `engage` is layer 7 and may import `job` (5) and `application` (6).

**Deploy** — a new unit and timer under `deploy/systemd/`, and the binary added
to the build list in `deploy/bin/release.sh`. That script lives on the host and
is not deployed by anything; the repository edit is only half the work.

**External** — a Discord incoming webhook URL, arriving through the two-file env
split on the host. Nothing else.

**Deferred, with the reason recorded so it is not rediscovered:** a LinkedIn
company-page publisher. The API exists (Posts API, under the Community
Management product) and carries no per-call fee — the Development Tier is free
and one post a day sits far inside its limits. Two things gate it, and neither
is money:

- **Review.** The access request is filed and pending. LinkedIn publishes no
  turnaround, and the product must be the *only* one on its application, so it
  lives in an app of its own.
- **Token lifecycle.** The access token lasts 60 days and the refresh token a
  year, so unattended posting needs a refresh worker that a Discord webhook does
  not. That worker is part of the follow-up change, not an afterthought to it —
  the first expiry would otherwise look exactly like a broken digest.
