# Telegram conventions

## Scope
Telegram-channel crawl (web preview → `telegram_posts`) and LLM vacancy extraction into the job catalogue.

## Always true
- `sources/telegram.yml` lists channels (each with a `kind` that steers the extraction prompt).
- `cmd/tg-ingest` crawls each channel's web preview into the `telegram_posts` queue.
- `cmd/tg-extract` drains pending posts through the LLM into the job catalogue.
- Both are run-once-and-exit cron workers.
- Crawl is cheap and LLM-free; extraction is the metered, retryable stage.

## How it works
Public Telegram channels carry vacancies as free-form posts, so unlike the structured ATS adapters they need an extraction step. The work is split into two stages mirroring the ingest/enrich shape: `cmd/tg-ingest` is the cheap crawl that fetches each channel's web preview and enqueues raw posts, and `cmd/tg-extract` is the LLM-driven extraction that drains the queue into normalized jobs. The `kind` field on each channel entry steers which extraction prompt is used, so different channel formats (e.g. a pure-vacancy channel vs a mixed discussion channel) get the right parsing strategy.

## Limitations
None currently listed.
