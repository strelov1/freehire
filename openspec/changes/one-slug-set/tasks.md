## 1. Check what is actually left

- [x] 1.1 Confirm the three bodies are identical before merging them, rather than trusting the
      finding's summary.
- [x] 1.2 Check catalogue #36 against the code first — it turned out to be already closed by S1,
      which the title-based mapping between the catalogue and the shortlist did not show.

## 2. Collapse the payload half

- [x] 2.1 `SlugSet` owns the set, `has`, `mark`, `unmark` and the two hooks; subclasses supply
      only `load()`.
- [x] 2.2 Put `unmark` on the base and say why — one of the three lacked it only because nothing
      called it.

## 3. Verify and close

- [x] 3.1 `pnpm run check` no worse than the baseline (0 errors, 18 warnings, 9 files).
- [x] 3.2 Record that no unit test is possible here, with the repo's own precedent for why.
- [x] 3.3 Mark #36 and #38 ✅ in `docs/reviews/2026-08-01-architecture-review.md`.
