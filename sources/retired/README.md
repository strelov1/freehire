# Retired boards

Board entries that are no longer crawled, kept rather than deleted so anyone who wants
them can pick them up.

A file here has the same shape as one in `sources/` — a list of `company` + `board`
entries — and is simply never read:

- **`cmd/ingest`** takes one board file by path (an argument, or `SOURCES_FILE`); it
  never scans a directory. Nothing here is passed to it, so nothing here is crawled.
- **`cmd/prune`** globs `sources/*.y*ml`, and a glob does not descend into
  subdirectories. An entry moved here therefore reads as retired, which is what the
  company-scoped pruning rules require before they may remove that company's jobs.

Both properties are pinned by a test in `cmd/prune`, because the second one is a safety
gate: if the glob ever became recursive, retired boards would silently read as live and
the rules that need a retired board would stop firing.

## How an entry gets here

`cmd/prune --boards` lists the boards whose postings were classified and none of them
came out technical — no technical title or category, and not one tagged **engineering**
skill, across their whole history. Move those entries out of `sources/<provider>.yml`
and into `sources/retired/<provider>.yml`, in the same PR that prunes their jobs.

Engineering is the operative word. The skills dictionary deliberately covers the
recruiting, HR, finance, legal, operations and customer-success craft a technical
company hires for, because the facet describes every posting. Counting any tag as
technical evidence let a recruiting coordinator vouch for a whole board; the report
asks `skilltag.HasEngineering` instead.

Boards no posting of which has been classified are withheld from that list and counted
at the top of the report. They are not safe to retire; nothing is known about them.
`is_tech` is tri-state and most of the catalogue carries no verdict, so a board whose
titles the dictionaries could not place shows no technical signal for want of any
signal — the first full run of this report named 11023 such boards out of 17841, and
among them live IT employers whose only listed posting was "Open Application". A
withheld board returns to the list when classification reaches it, which is what
expanding the dictionaries (see `cmd/mine-titles`) is for.

Order matters: **prune first, then move the provider's last entry.** Once a provider has
no entries left in `sources/`, none of its jobs are re-crawlable, and every pruning rule
refuses them — the dead weight becomes permanent.

The report enforces the reminder rather than leaving it here: when its list covers every
board a provider has, it prints a `CAUTION` line naming that provider. The entries are
still genuine candidates — move them, but move the ones that empty a provider last, once
its jobs are gone.

`cmd/prune --retire` performs the move. It computes the same list the report prints (both
go through one function, so the list you read and the list it acts on cannot diverge),
edits the files line by line so their headers and group comments survive, and **refuses**
the entries that would empty a provider — naming them, so the refusal reads as "later",
not as "nothing to do". Review the diff; that is the gate.

## Why keep them

They cost nothing, they record what was considered and rejected, and a board that turns
out to have been retired by mistake is restored by moving one line back.
