## Context

A spike confirmed the per-job remote signal is NOT in the search listing (the `homeoffice` facet is
aggregate-only), but the detail page's `ng-state` JSON carries `jobdetail.homeofficemoeglich`
(boolean) alongside the `stellenangebotsBeschreibung` the adapter already reads. Sampled remote
postings return `homeofficemoeglich: true` with `homeofficetyp: "NACH_VEREINBARUNG"`.

## Decisions

- **Map the boolean home-office flag through `workModeFromRemote`.** `homeofficemoeglich: true` →
  `Remote: true`, `WorkMode: "remote"`; `false`/absent → left unset. This mirrors `apple.go`, which
  maps its own `HomeOffice` boolean the same way, so the codebase already treats a home-office flag
  as "remote". *Alternative rejected:* mapping `homeofficetyp` to `hybrid` vs `remote` — the type is
  overwhelmingly `NACH_VEREINBARUNG` (by arrangement) and adds a bespoke sub-map for no filter-level
  benefit; the boolean is the signal users filter on.

- **Reuse the existing detail fetch.** The description already parses `ng-state`; the detail struct
  gains one field (`homeofficemoeglich`) and `toJob` sets remote/work mode from the same parse. No
  extra request, no new failure mode — a failed detail still yields the posting (now simply without
  the remote flag, as before it lacked a description).

## Risks / Trade-offs

- **"By arrangement" ≠ fully remote.** Flagging it `remote` is slightly generous, but matches the
  apple precedent and the user intent (home-office-possible German jobs should be findable under the
  remote filter). Accepted.

## Migration Plan

- No DB migration, no API change. A re-crawl re-derives the flag for existing arbeitsagentur rows.
