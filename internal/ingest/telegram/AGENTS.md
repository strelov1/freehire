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
- The prefilter's marker set is per-language and hand-maintained (RU, EN, UA, ES). Adding a
  channel that publishes in a language the markers do not cover silently rejects all of its
  vacancies — the failure looks like a weak channel, not a blind filter. Extend
  `internal/ingest/telegram/prefilter.go` before adding the channel.
- **That blindness is measured, not hypothetical, and a marker list cannot close it.**
  Of the 15,203 posts the filter had rejected as at 2026-09-23, the Spanish cohort alone was
  6,340 — 42% — and the `empresa:` marker added that day recovers 6,335 of them. What remains
  rejected is **not one more language away**:

  | channel | rejected | why the markers miss it |
  |---|---|---|
  | `seekingyourjobs` | 606 | Persian/Dari (`اعلان کاریابی`) |
  | `morejobs` | 594 | **Russian** — a covered language; "приглашает в команду" is not a marker |
  | `amalw3amal1` | 598 | Arabic (`المنصب الوظيفي`) |
  | `huntmejob` | 443 | Uzbek (`Lavozim`, `Ish turi`) |
  | `DeJob_official` | 380 | Chinese (`#招聘`) |
  | `Remoteit` | 336 | **English, no hiring verb at all** — a title plus tags |
  | `forproducts` | 321 | **English** — a plain job description |

  The last two are the argument: `QA AUTOMATION C# | REMOTE RUSSIA | SIMBIRSOFT #remote
  #fulltime` is a real vacancy in a covered language holding no phrase a keyword could
  select without also selecting every other title-shaped post. Closing this needs a
  classifier, not a longer list. Not all of the rejected set is loss — `gamedev_dou` (286,
  Ukrainian gaming news) and `jobnetworkng` (433, interview-advice articles) are refused
  correctly.
- **A rejected post is never re-examined, so widening the markers does not reach the ones
  already stored.** `InsertTelegramPost` stamps `extracted_at` at insert time for a post the
  prefilter declined, and nothing revisits it. Measured the same day: 96 rejected posts
  (all `job_it_junior`) already match today's marker set — they were crawled under a
  narrower one and are stuck. **`cmd/backfill-telegram-prefilter` is the answer, and it is
  worth running after any marker change**: it pages the declined posts, re-asks
  `telegram.AdmitsPost`, and clears `extracted_at` on the ones today's rule admits. Reports
  by default, writes under `--apply`, bounded by `BACKFILL_TG_PREFILTER_MAX` — and
  **resumed with `BACKFILL_TG_PREFILTER_AFTER_CHANNEL` / `_AFTER_MSG_ID`, which is what
  makes that bound a bound**: a refused post never leaves the predicate, so a second run
  starting from the top rescans exactly what the first one rejected, and a dry run — where
  nothing leaves the predicate at all — repeats its report forever. The two are ONE cursor
  and the run refuses half of it; they are EXCLUSIVE, hence `AFTER` and not the sibling
  passes' inclusive `FROM`.
- Telegram jobs have no close signal of their own: the ingest sweep does not reach them, there
  is no change feed, and `cmd/liveness` excludes them from the probe because the stored URL is
  the post, which outlives the vacancy. They are closed by age instead — 45 days on
  `COALESCE(posted_at, created_at)`, `closed_reason = 'expired'`. That is a guess, not
  evidence: a vacancy still open at 46 days is closed anyway.

## The admission rule has exactly one home
`telegram.AdmitsPost(text, links, matcher)` — the text carries a marker, OR the post links
out to a vacancy a destination adapter resolves — is called by BOTH `CrawlRunner` and
`cmd/backfill-telegram-prefilter`. It was a line inside the crawl until the backfill needed
it, and the two disagreeing is not cosmetic: the backfill would requeue posts the next
crawl refuses, or leave behind the ones it now admits. Add a marker and both readers move
together. A nil matcher (no registry configured) means only the text can admit.
