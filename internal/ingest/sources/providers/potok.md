**Potok traps** (`potok`; verified live 2026-10-14):

- **`*.potok.io` is a wildcard in DNS and in the certificate**, so neither names a tenant; only `GET <slug>.potok.io/open/jobs/` does (200 against 404). The tenant list was found by probing guessed slugs (780 tenants, 130 with live postings).
- **A tenant with nothing open still renders `.postings-wrapper`, empty.** A 200 without it is not the career page, and `list` fails the board rather than reading it as empty.
- **The listing groups postings, and the group is a department — except on a hub.** On `tehnopark-skolkovo` each group is a resident company, so that board must be added as a hub (`cmd/add-board --hub`); the adapter then files each posting under its group and falls back to the configured company for a blank one.
- **Neither page states a salary or a publish date.** The posting page's headline renders the city and the schedule as two `sort-by-time` categories in that order; the schedule is an hh-style picklist and only `Удаленная работа` says anything about the arrangement, so only it sets `WorkMode`.
- **No throttling at ~12 req/s from 3 workers**, the only rate measured, so `limitedPotokGetter` holds the run to 3 in flight.
