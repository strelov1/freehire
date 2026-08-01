## Why

`JobDrawer.svelte` re-implements two `$lib` modules whose stated job is to be the single home for
exactly that code.

`JobDescription.svelte` says so in its own first line — "one home for the CSS so the description
reads the same everywhere" — and the drawer carries a **byte-identical 39-line copy** of its
style block instead of rendering it. The two are one edit away from disagreeing, and the copy's
comment already points somewhere false: it says "Styles mirror JobView's `.job-description`", but
JobView holds no such rule any more — it renders `<JobDescription>`. That is how the next person
misses the second copy.

`scrollLock.ts` is a reference-counted body lock, so two overlays can hold it at once and the
body only unlocks when the last releases. The drawer hand-rolls the lock instead, saving and
restoring `document.body.style.overflow` directly. Not a live defect — the drawer is
`fixed inset-0 z-50`, so it covers the header and the menu that would contend for the lock cannot
be opened over it — but a direct write is precisely what breaks a refcount that only acts on the
0↔1 transition, so the exemption depends on a layout fact rather than on the lock's contract.

## What Changes

- The drawer renders `<JobDescription html={item.job.description} />` and its 39-line
  `.job-description` style block is deleted. The surrounding `{#if}` branch is unchanged, so the
  "No description available." fallback still reads the same.
- The drawer's `$effect` calls `lockScroll()` / `unlockScroll()` instead of writing
  `document.body.style.overflow` itself.
- No behaviour change and no visual change: the CSS moving is byte-identical and the wrapper div
  carries the same classes (`job-description text-sm leading-relaxed`).

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

(none) — no requirement-level behaviour changes. This is a reuse cleanup; `tasks.md` is the real
artifact, and the change archives with `--skip-specs`.

## Impact

- `web/src/lib/components/JobDrawer.svelte` — one import, one element, one `$effect`, minus 39
  lines of CSS.
- No backend change, no contract change, no migration.
