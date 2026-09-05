## Context

`cmd/rollup-views` already counts, per day and per job, two distinct signals it
takes from the nginx access log: a page open (`GET /jobs/<slug>` and its
SvelteKit `__data.json` twin, filtered against a known-bot list) and an API read
(`GET /api/v1/jobs/<slug>`, deliberately **not** bot-filtered — the API is meant
to be read by programs). `viewlog.Aggregate` sums the two into one number per
`(day, slug)` and that number becomes `job_daily_views.uniques`.

For the transparency figure that number serves today, fusing them is fine. For
ranking a post we publish under our own name, it is not: AI crawlers are the
majority of this host's traffic, and the bot list in `viewlog/bot.go` is
deliberately small — it errs toward counting a person, not toward excluding a
crawler. A "most viewed today" list built on `uniques` would be a list of what
crawlers fetched, presented to humans as what humans liked.

The rest is plumbing this repository already has a shape for: a run-once worker
under `cmd/`, a domain package under `internal/engage`, a systemd oneshot and
timer under `deploy/systemd/`.

## Goals / Non-Goals

**Goals:**

- Materialize the page-open signal separately, without moving the value that
  `GET /api/v1/stats/catalog` reads.
- Publish one honest, readable daily post to a Discord channel, on a schedule,
  unattended — behind a seam that takes a second destination without reshaping
  anything.
- Make the run safe to repeat: a second run for the same day publishes nothing.
- Make the rendered post inspectable before it is ever sent.

**Non-Goals:**

- **Backfilling `page_uniques` over history.** The `.gz` log history exists and
  could be re-read, but the digest only reads the freshest day. Re-reading months
  of logs to fill a column nothing queries would be work for its own sake.
- **A LinkedIn publisher.** The Community Management API access request is filed
  and awaiting review, with no promised date. Until it clears there is no
  organization URN and no token, so the publisher could be neither configured nor
  pointed at anything real — it would be code that ships and never runs. The API
  itself is free (Development Tier, no per-call fee), so none of this is about
  cost. When the credentials exist the work is one `Publisher` plus a
  token-refresh worker: the access token lasts 60 days, and a webhook needs no
  such thing, which is why the refresh path belongs to that change rather than
  being retrofitted onto this one.
- **A web surface for the digest.** No page, no archive, no feed. If the archive
  turns out to be wanted, `social_digest_posts` already holds it.
- **Per-audience variants.** One list, rendered per channel's format. Not one
  list per region or per category.

## Decisions

### Add a column rather than change what `uniques` means

`uniques` keeps counting both signals. `page_uniques` is added beside it.

The alternative — redefining `uniques` as page-only and adding `api_uniques` —
reads better on a blank page, but `uniques` is read by
`catalogstats` and by the archived `view-count-aggregation` spec, and its value
is already accumulated across months. Redefining it would silently restate a
public figure and would require a backfill to make the old rows agree with the
new meaning. Adding a column beside it changes nothing that already works.

### `Aggregate` returns a struct, not a second map

`Aggregate` currently returns `map[day]map[slug]int`. It will return
`map[day]map[slug]Counts` where `Counts` holds `Total` and `Page`.

The alternative — returning two parallel maps — makes it possible for a caller
to walk one and index the other, which is exactly the bug this split exists to
prevent. One value per `(day, slug)` cannot drift apart.

Dedup stays where it is: the existing `(IP, UA, slug, day)` tuple already
collapses repeats, and the page/API distinction is a property of the signal, not
of the visitor. A visitor who both opens the page and calls the API on the same
day counts once in each — which is what "uniques per signal" means.

### Eligibility rides the repository's existing open-posting predicate

`closed_at IS NULL AND duplicate_of IS NULL AND NOT is_private` is the predicate
every public listing already uses, and `duplicate_of` is derived from the three
owned marker columns (migration 0115), so it covers all three markers without
naming them. The digest adds one term: `ats_absent_at IS NULL`, so a posting the
source's own ATS has stopped listing is never promoted.

The full ghost verdict (`internal/job/ghost`) is a hedged classification that
needs evidence the digest query has no reason to gather. `ats_absent_at` is the
strongest single piece of that evidence and is a plain column.

### The day is discovered, not computed

`cmd/rollup-views` fires at 02:30 UTC and reads `access.log.1`. Whether that
file is yesterday or the day before depends on when logrotate runs on the host —
which is not recorded anywhere in this repository and must be checked on the
machine. Rather than encode an assumption that would fail silently by publishing
a two-day-old list, the digest asks the database for the freshest day it has.

The staleness guard (fail if that day is more than three days old) is what keeps
"discover the day" from degrading into "publish whatever is there" when the view
pipeline breaks. Three days, not one, because `rollup-views` missing a single
night is normal operational noise.

### Publishers are an interface, and the ledger is per-channel

```go
type Publisher interface {
    Name() string
    Render(d Digest) (string, error)
    Publish(ctx context.Context, d Digest) error
}
```

`Render` was not in the first sketch of this seam and the dry-run requirement put
it there. A dry run exists to catch a list that **reads** badly, and a summary of
a post cannot read badly — so the thing a dry run prints has to be the payload
itself, which only the publisher can build. `Publish` calls it too, so the two
cannot drift.

Three more things the implementation added that this design did not ask for, each
because publishing under our own name has a lower tolerance than the sketch
assumed:

- **`?utm_source=<channel>` on every link.** The digest is a marketing channel; if
  its traffic is not separable from every other inbound path, nobody can say
  whether it worked.
- **The worker refuses to publish when `FRONTEND_ORIGIN` is not `https://`.** That
  variable defaults to `localhost:5173` so a developer's checkout runs, and every
  link in the post is rooted at it. This is the only worker in the fleet whose
  output strangers read, so the default costs ten dead public links rather than a
  quiet local oddity. A dry run is exempt.
- **HTML-entity unescaping, Discord markdown escaping, and truncation at
  Discord's own description limit.** All three were found by rendering real
  production rows: `Learning Design &amp; Capacity` is a real title, and this
  catalogue's titles are unbounded.

The ledger key is `(day, channel)`, not `(day)`. A run that posts to Discord and
then fails on a second channel must, on its next attempt, skip Discord and retry
the other. Keying on the day alone would either re-post to Discord or abandon the
second channel, and both are wrong.

That only one channel exists today does not make the key premature: a ledger keyed
on the day is a schema change away from a second channel, and this one is not.

The quarantine reads the same ledger across channels: a posting shown on Discord
yesterday is quarantined everywhere today. The list is the editorial unit;
the channel is only how it is delivered.

### The floor and the quarantine are constants

Ten page uniques and seven days. Both are editorial judgements about what the
public sees, and both are guesses until a week of `-dry-run` output says
otherwise. Constants in the package mean changing them is a commit that is
reviewed; env vars would mean changing them is an SSH session nobody sees.

### The timer names the timezone

`OnCalendar=*-*-* 10:00:00 America/Sao_Paulo` rather than `13:00:00` UTC. Brazil
has had no DST since 2019, so today the two are the same instant. If that
changes, the named zone follows and the hard-coded UTC hour silently drifts by
an hour. The unit costs nothing to write correctly.

13:00 UTC is clear of this host's heavy cron block (03:00–10:00 UTC), and this
worker is one query and two HTTP calls regardless.

## Risks / Trade-offs

- **`page_uniques` is zero for every historical row** → The digest never reads
  them. Anything later that wants historical page/API split must re-run
  `rollup-views --backfill` against a cleared cursor, which is possible but is
  not this change's problem.
- **A one-channel dispatcher is a dispatcher nobody has exercised.** The
  multi-channel behaviour this design specifies — attempt every channel, let one
  fail without skipping another — has no second channel to prove it in
  production → It is covered by tests with two fake publishers, which is where
  that behaviour is worth proving anyway. The alternative, hard-coding a single
  publisher and generalizing later, would leave the ledger keyed on the day and
  make the second channel a schema change.
- **A publish that succeeds but whose ledger write fails** would re-post the next
  day → The ledger write and the publish cannot be one transaction across an
  HTTP boundary. The ledger is written immediately after a successful publish,
  and the residual window is one failed database write wide. Accepted: the cost
  is one duplicated post, and the alternative (write first, publish after) risks
  a day that is silently never published, which is worse and harder to notice.
- **Ten postings from a thin day could still look thin** even above the floor →
  The floor is a constant precisely so the first week of dry runs can move it.
- **`viewlog.Aggregate`'s signature change touches every caller** → There is one
  production caller (`cmd/rollup-views`) plus tests. `go build ./...` finds them
  all; this is a compile error, not a silent one.

## Migration Plan

1. Migration `0138` adds `job_daily_views.page_uniques` (`NOT NULL DEFAULT 0`)
   and creates `social_digest_posts`. Both are additive; nothing reads the new
   column until the digest ships.
2. Ship the `viewlog` split and the `rollup-views` write. From the next nightly
   run, `page_uniques` is correct for new days.
3. Ship the selection package and the worker with **no timer installed**. Run
   `-dry-run` by hand for a week and read the lists.
4. Configure the Discord webhook, install the unit and timer, enable Discord.
5. Read the dry runs for a week and move the floor or the quarantine if they are
   wrong. Only then is the change finished.

**Rollback:** stop the timer. The column and the ledger are inert without the
worker; nothing else reads them.

**Deployment note that is easy to miss:** `deploy/bin/release.sh` holds the list
of worker binaries to build, and the copy that runs lives on the host. Editing
the repository copy does not deploy it — the host's copy must be updated by hand
or the new binary is simply never built.

## Open Questions

- ~~When does logrotate run on the production host, relative to `rollup-views` at
  02:30 UTC?~~ **Answered 2026-09-05 on the host:** `logrotate.timer` fires at
  **00:00 UTC**, two and a half hours before `freehire-rollup-views.timer` at
  02:31 UTC, and `access.log.1` carries the day that just ended. So the freshest
  day in `job_daily_views` is **yesterday**, and the 13:00 UTC post is about
  yesterday.

  This does not retire the discover-the-day design. It cost nothing, and what it
  protects against is precisely a schedule nobody will remember is load-bearing:
  a logrotate moved past 02:31 would silently make every post a day older, and
  the failure would look exactly like a working digest.
- **The floor (10) and the quarantine (7 days)** are starting values, to be
  revisited after the week of dry runs in step 3.
