## 1. Defer, never store body-less

- [x] 1.1 A posting whose detail request fails or returns no content is dropped from the run instead
      of being ingested list-only. Test: replaces the shipped
      `TestSeekFailedDetailKeepsTheListOnlyPosting`, asserting both the transport-error and the
      empty-content case yield no job, while a posting that hydrates is unaffected and a `SeenRefresh`
      posting still passes through without a detail request.

## 2. Pace the detail fetch

- [x] 2.1 Add a rate-limited `JSONPoster` to `internal/sources/pacer.go` alongside the existing HTML
      and JSON wrappers, plus the SEEK interval/burst constants carrying their rationale. Test: the
      wrapper waits on its limiter before delegating, and a cancelled context surfaces the wait error
      without issuing the request.
- [x] 2.2 Wire the paced poster into `NewSeek` in `sources.All` so one limiter is shared by every
      board in a run, leaving the search listing on the bare client. Test: the adapter still resolves
      from the registry and its boards validate.

## 3. Documentation

- [x] 3.1 Record the rate-limit finding in `internal/sources/AGENTS.md` — the burst window, the
      measured refusal, the two-minute recovery, and why SEEK alone defers a posting rather than
      ingesting it body-less.

## 4. Production recovery

- [ ] 4.1 Deploy, then `cmd/prune` the `seek` rows so the paced crawl re-ingests them complete.
- [ ] 4.2 Run the paced crawl and confirm the description-fill rate and the refusal count before
      enabling the hourly timer.
- [ ] 4.3 Enable `freehire-ingest@seek.timer` once a run has verified clean.
