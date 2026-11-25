# remoteok

Verified live 2026-11-24. The `/api` feed is the newest 100 postings only: `offset=` is ignored and `?tag=X`
is another 100-cap window per tag (14 tags still left 137 of 452 live postings unreachable). Every live
posting is on the site's own "load more" endpoint, `/?action=get_jobs&offset=N` (N a row index, 50 rows a
page, empty past the end; it answers bare `<tr>` rows, so wrap them in a `<table>` before parsing). That
listing carries each body in `div.html` (the poster's HTML) or `div.markdown` (plain text with the newlines
written as the two characters `\n`), but its salary label is `$80k - $160k` (RemoteOK's placeholder) or "Upgrade
to Premium", and the page JSON-LD body is cut at ~1000 characters, so salary comes from the API only.

- A posting inside the API window is mapped from the API; the rest from the listing. ExternalID is the numeric
  id in both, so rows do not duplicate.
- The API serves UTF-8 read as Latin-1 ("MÃ¼nchen"); `remoteokUnmojibake` repairs it.
- Empty API location is RemoteOK's "Probably worldwide"; the adapter says "Worldwide".
- `FetchNew`: a listing-only posting already stored is only re-listed (`SeenRefresh`), so its API-quality body
  is not replaced by the listing's plain text.
- robots.txt allows `/` for generic agents; ~12 requests, ~5 MB per run, paced by the shared client.
