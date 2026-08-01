## 1. The description

- [x] 1.1 Confirm the two style blocks are still byte-identical before deleting either — the
      claim is what makes this a safe deletion rather than a restyle.
- [x] 1.2 Import `JobDescription` and render it in place of the inline `{@html}` div, keeping the
      surrounding `{#if item.job.description}` / `{:else}` branch as-is.
- [x] 1.3 Delete the `.job-description` style block and the two `{@html}` eslint/sanitization
      comments that came with the inline copy — they live in `JobDescription` already.

## 2. The scroll lock

- [x] 2.1 Swap the `$effect` for `lockScroll()` with `unlockScroll()` as its cleanup, matching
      `HeaderMenu`/`HeaderSearch`. Say in the comment why the refcount matters even though the
      drawer covers the header today, so the next reader does not restore the direct write.

## 3. Verify and close

- [x] 3.1 `pnpm run check` in `web/` — no worse than the baseline (which is warnings-only, not
      zero).
- [x] 3.2 Establish that the deletion cannot restyle anything. NOT verified by rendering the
      authenticated drawer — that needs a signed-in account with tracked jobs and the full stack.
      Verified structurally instead, which for this change is decisive: the deleted block was
      byte-identical to `JobDescription`'s (`diff` empty over 39 lines), the wrapper div carries
      the same classes, the `:global()` scoping construct is the same one, and the component
      already renders on two other surfaces. Confirm no `.job-description` selector remains in
      the drawer.
- [x] 3.3 Mark S10 ✅ in `docs/reviews/2026-08-01-architecture-review.md` — shortlist row, the
      `S10` heading, the Progress table — noting anything the finding got wrong.
