**FriendWork traps** (`friendwork`; verified live 2026-10-14):

- **`page`/`perPage` are ignored** — the response says `perPage: 20` and carries 151 postings, and `page=2` returns the same 151 — so one request is the whole board.
- **The API meters simultaneity:** two parallel workers drew 429s, sequential ~2 req/s did not. `limitedFriendWorkGetter` runs one request at a time at that pace; harvest probes through the same registry, so they inherit it.
- **Salary is monthly by the platform's own rendering** (`<from> <to> <currency>/month`, read off the portal bundle), so `SalaryPeriod` is `month`; `0` means unstated, and `currency` is a numeric enum (0 RUB … 7 UZS).
- **Unset enums are the STRING `"-1"`**, not a number or null; they map to nothing. Dates carry no zone and are read as UTC.
- **Tokens are case-insensitive** (`TeDo` = `tedo`); the catalogue's unique index on `lower(board)` already folds them. Many portals still list postings years old — the platform says they are published, and the listing is the only evidence there is.
