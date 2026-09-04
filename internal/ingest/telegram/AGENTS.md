# Telegram conventions

## Scope
Telegram-channel crawl (web preview → `telegram_posts`) and LLM vacancy extraction into the job catalogue.

## Always true
- The `telegram_channels` table lists channels (each with a `kind` that steers the extraction prompt); `active=false` retires one without losing its posts.
- `cmd/tg-ingest` crawls each channel's web preview into the `telegram_posts` queue.
- `cmd/tg-extract` drains pending posts through the LLM into the job catalogue.
- Both are run-once-and-exit cron workers.
- Crawl is cheap and LLM-free; extraction is the metered, retryable stage.

## How it works
Public Telegram channels carry vacancies as free-form posts, so unlike the structured ATS adapters they need an extraction step. The work is split into two stages mirroring the ingest/enrich shape: `cmd/tg-ingest` is the cheap crawl that fetches each channel's web preview and enqueues raw posts, and `cmd/tg-extract` is the LLM-driven extraction that drains the queue into normalized jobs. The `kind` field on each channel entry steers which extraction prompt is used, so different channel formats (e.g. a pure-vacancy channel vs a mixed discussion channel) get the right parsing strategy.

## Limitations
- The prefilter's marker set is per-language and hand-maintained (RU, EN, UA). Adding a
  channel that publishes in a language the markers do not cover silently rejects all of its
  vacancies — the failure looks like a weak channel, not a blind filter. Extend
  `internal/ingest/telegram/prefilter.go` before adding the channel.
- Telegram jobs have no close signal of their own: the ingest sweep does not reach them, there
  is no change feed, and `cmd/liveness` excludes them from the probe because the stored URL is
  the post, which outlives the vacancy. They are closed by age instead — 45 days on
  `COALESCE(posted_at, created_at)`, `closed_reason = 'expired'`. That is a guess, not
  evidence: a vacancy still open at 46 days is closed anyway.
