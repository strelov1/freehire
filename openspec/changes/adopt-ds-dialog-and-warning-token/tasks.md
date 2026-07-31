## 1. The warning token family

- [ ] 1.1 Add `warning`, `warning-foreground`, `warning-strong`, `warning-muted` to
      `tokens/color.tokens.json` and `tokens/color-dark.tokens.json`, in oklch, each with the
      `$description` the other entries carry. Values chosen against the amber shades the app
      already ships, so nothing gets lighter or darker than it is today.
- [ ] 1.2 `pnpm build`, and commit `dist/` in the same commit — CI now diffs it.
- [ ] 1.3 Confirm the four utilities exist: `bg-warning`, `text-warning-foreground`,
      `text-warning-strong`, `bg-warning-muted` resolve in a scratch story. A token that mints
      no utility fails silently.
- [ ] 1.4 Update the `foundation.json` DSDS entity — `validate:docs` compares its token list
      against the file in both directions and will fail otherwise.

## 2. Amber → warning, 21 files

- [ ] 2.1 Apply the mapping table from design.md. Delete every `dark:` amber variant rather
      than translating it; the token is what `.dark` overrides.
- [ ] 2.2 Sweep for stragglers: no `amber` left under `web/src`.
- [ ] 2.3 `pnpm check:tokens` — the count drops. Record how far with `--update`.
- [ ] 2.4 **Visual check, both themes**, of the surfaces that carry the most: `GhostBadge`,
      `RealityBadge`, `GhostChecklist`, `MatchAnalysisFull`, `StatusBoard`, `VerdictView`,
      `ATSReportView`. A wrong lightness compiles perfectly.

## 3. Five modals onto Dialog

For each of `AuthDialog`, `ReportDialog`, `GmailConnectDialog`, `RequestReferralModal`,
`DeleteAccountButton` — one at a time, not as a batch:

- [ ] 3.1 Read its current close path first — Escape, backdrop, cancel button — and note what
      each does beyond closing. A handler that also resets form state does not survive on its
      own terms.
- [ ] 3.2 Replace the overlay with `<Dialog bind:open title description>`; drop the
      `fixed inset-0`, the z-index, the Escape handler and the hand-rolled focus handling.
- [ ] 3.3 Width via `class` where the default `max-w-lg` is wrong.
- [ ] 3.4 Verify by hand: opens, closes on Escape, closes on backdrop, closes on its own
      button, submits, and focus returns to the trigger. Both themes.
- [ ] 3.5 `pnpm check:adoption` — `Dialog` rises; `--update` and commit the diff.

## 4. Record what was not done

- [ ] 4.1 One paragraph in `design-system/AGENTS.md`: four surfaces want a Sheet, which the
      system does not have, and why they were not bent onto `Dialog`.
- [ ] 4.2 Note `CookieConsent`'s `role="dialog"` on a non-modal banner as an accessibility
      defect to fix separately — not in this diff.
- [ ] 4.3 Add the adjacent palettes and their counts (success 41, danger 32, informational 23)
      where the next change will find them.

## 5. Finish

- [ ] 5.1 Rebase onto main and re-run **both** `--update`s immediately before merge; a
      baseline that went stale mid-review is expected, not a defect.
- [ ] 5.2 Full local run of the `design-system` job, every check green.
- [ ] 5.3 Review the whole diff; act on Critical and Important.
- [ ] 5.4 Integrate, then `/opsx:archive` and `/opsx:sync`.
- [ ] 5.5 User-facing? The colour is. Offer a changelog entry.
