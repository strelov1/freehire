# Join

The board is the numeric company ID. Its public jobs API caps `pageSize` at 5.
Read every declared page, require an explicit `items` array and nonnegative
`rowCount`/`pageCount`, and reject changing totals, repeated identities, malformed
items, or a final count different from the declared row count. A failed later
page fails the entire listing; no page cap may silently truncate it.

Verified on 2026-11-11: company 176781 returns `items: []` and pagination
`rowCount: 0, pageCount: 0, page: 1, pageSize: 0`. The unknown company
999999999999 returns HTTP 404 rather than this empty envelope. These responses
justify the full and empty board listing markers.

The native posting ID differs from the display ID in its public URL. Detail
requests use the native ID. A detail 404/410 proves removal; transport failures
and missing bodies retain an unreadable identity, protecting that listed job
from the unseen sweep.
