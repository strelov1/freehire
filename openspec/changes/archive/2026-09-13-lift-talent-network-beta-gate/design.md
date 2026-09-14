## Context

See proposal.md - Why. The gate is a single `IsBetaTester` check in
`talentNetworkHandlers.PutVisibility` (`internal/api/handler/me_talent_network.go:133-140`),
guarding only the non-`off` branch (leaving is already ungated). The two client-side
mirrors are `web/src/lib/components/profile/TalentNetworkInvite.svelte`'s `beta` derived
value (hides the whole invitation card) and `web/src/lib/accountNav.ts`'s `betaOnly: true`
on the Talent Network item (hides the nav entry via `visibleAccountNav`). `noindex` on
`/talent` is a hardcoded `<meta>` tag, not conditional on anything — it was never wired to
`beta_tester`, just written for the beta period and left for this change to lift per its
own comment.

## Goals / Non-Goals

**Goals:**
- Every signed-in candidate, not only beta testers, can join and appear in the catalogue.
- The catalogue list becomes indexable.
- One entry point into membership (the profile page) instead of two.

**Non-Goals:**
- Changing anything about the catalogue projection, the handle, or the card page itself.
- Touching `users.beta_tester`, `IsBetaTester`, or any other feature still gated on it
  (Mentorship's nav entry, `visibleAccountNav`'s `betaOnly` mechanism itself).
- A migration or backfill — no stored value changes shape or meaning.

## Decisions

**Delete the nav item rather than drop only `betaOnly` from it.** Dropping just the flag
would leave two entry points (nav + profile card), which is what the proposal is
retiring. `visibleAccountNav`'s generic `betaOnly` filter mechanism stays — Mentorship
still uses it — only the Talent Network row is removed from `accountNav`.

**Remove the `IsBetaTester` check and dependency rather than short-circuit it.** Leaving
`talentNetworkStore.IsBetaTester` wired but unused would be dead code the moment this
merges (`internal/platform/db`'s `IsBetaTester` query itself stays — `assistant.go` and
`cv.go`'s history show other features have carried and then dropped their own beta
checks against the same underlying query, so the query staying live is normal, not
orphaned). Delete the interface method, the field read, and the 403 branch together.

**`noindex` comes out of `talent/+page.svelte` entirely, not behind a flag.** There is no
successor condition to gate it on — the requirement this change retires is what put it
there, and `talent-network-catalog`'s existing "The list is indexable, a card is not"
requirement already says the list should never carry it.

## Risks / Trade-offs

- **A candidate who bookmarked `/my/talent-network` while it was gated keeps working** —
  the route itself is untouched, only its nav entry moves. No risk.
- **Search engines indexing the list immediately** is the intended effect, not a
  side-effect to mitigate.
- **Losing the second entry point could cost membership growth if the profile card is
  ever hidden or broken** — mitigated by the profile card already being covered by its
  own tests and being the page every candidate visits to finish describing themselves
  (proposal.md - Why).
