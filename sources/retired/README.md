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

`cmd/prune --boards` lists the boards whose companies have never posted anything
technical — no technical title or category, and not one tagged skill, across their whole
history. Move those entries out of `sources/<provider>.yml` and into
`sources/retired/<provider>.yml`, in the same PR that prunes their jobs.

Order matters: **prune first, then move the provider's last entry.** Once a provider has
no entries left in `sources/`, none of its jobs are re-crawlable, and every pruning rule
refuses them — the dead weight becomes permanent.

## Why keep them

They cost nothing, they record what was considered and rejected, and a board that turns
out to have been retired by mistake is restored by moving one line back.
