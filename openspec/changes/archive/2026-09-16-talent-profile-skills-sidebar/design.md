## Context

`web/src/routes/talent/[handle]/+page.svelte` (the only file this change touches) is
today a flat, single-column layout: a `max-w-3xl` wrapper (line 58) with sections
stacked via `flex flex-col gap-6` — header, "Open to", "Experience" (each role rendered
in a `Card` primitive), "Education", "Certifications", "Skills" (lines 155-164, a flat
`Chip` list), then a footer disclaimer.

The job view page (`web/src/lib/components/JobView.svelte`) already establishes this
product's two-column detail-page pattern: a `max-w-6xl` container
(`web/src/routes/jobs/[slug]/+page.svelte:62`) and a responsive grid
(`JobView.svelte:635-637`) — `flex flex-col gap-4 lg:grid
lg:grid-cols-[20rem_minmax(0,1fr)] lg:grid-rows-[auto_auto_minmax(0,1fr)]
lg:gap-x-6 lg:gap-y-4` — with a sticky left sidebar (`JobView.svelte:767-772`):
`sticky top-20 flex flex-col gap-4 rounded-xl border border-border bg-card p-4`,
carrying an uppercase section label (`text-xs font-semibold uppercase tracking-wide
text-muted-foreground`, `JobMatch.svelte:294-298`).

`card.skills` is already fetched and rendered on the talent page today (line
155-164) — this change only repositions it, no data/API change.

See proposal.md - Why / What Changes for the motivation and scope.

## Goals / Non-Goals

**Goals:**

- Match the talent card page's container width/padding and two-column grid mechanics
  to the job view page's, reusing the same Tailwind classes rather than inventing new
  sizing tokens.
- Give Skills a sidebar position visually consistent with the job page's "Profile
  match" box (same border/radius/padding/sticky/label style).
- Keep the page's mobile (stacked) reading order sensible for a profile: header first,
  Skills last — not copy the job page's own mobile order verbatim.

**Non-Goals:**

- No change to what data is fetched, computed, or sent to the client (`+page.server.ts`
  and the `CandidateCard` payload are untouched).
- No avatar/photo work (explicitly deferred by the user).
- No componentization of the "sidebar card" or "uppercase label" styles into a shared
  design-system primitive — the job page itself hand-rolls these same classes
  (`JobView.svelte:772`) rather than using the generic `Card` primitive, and this
  change follows that existing (uncomponentized) convention rather than introducing a
  new abstraction the rest of the codebase doesn't use yet.

## Decisions

**Grid placement: Skills as the `lg:col-start-1` sidebar, everything else in
`lg:col-start-2`.** This mirrors the job page's column assignment (facts sidebar left,
primary content right) directly, rather than inventing a different split (e.g. header +
Skills left, rest right) — there's exactly one sidebar-worthy list on this page today,
so the mapping is unambiguous.

**Mobile order via DOM position, not CSS `order`.** The job page's `aside` precedes
its content `div` in markup, so on mobile (`flex-col`, no grid) the match box renders
above the job title. Reusing that DOM order here would put "Skills" above the
candidate's own heading, which reads badly for a profile (the person before their
stack). Keeping the Skills block last in the DOM achieves both outcomes with no
`order` utility needed: below `lg` the container is `flex flex-col`, and with neither
item carrying an explicit `order`, flex falls back to document order, so Skills
renders last; at `lg:` both the sidebar and the content column carry a fully-explicit
grid-column (and the sidebar an explicit `grid-row` too), so CSS Grid places each
where its explicit coordinates say regardless of DOM position — `lg:col-start-1` puts
Skills in the first column either way. An `order-last lg:order-none` pair was tried
first on the assumption that `order` was doing the work; verified (via a
code-reviewer pass and a follow-up screenshot with the classes removed) that it was
inert in both states given this markup, and dropped as dead weight rather than kept
as defensive styling. Alternative considered: physically moving the Skills markup to
the top of the file, matching JobView's DOM order — rejected because it would also
change the mobile order, which is
not what was asked for or wanted.

**Sidebar card reuses `JobView.svelte:772`'s exact classes, not the `Card`
primitive.** `Card` (`design-system/src/card.svelte`) renders `rounded-lg ... shadow-sm`
— visibly different from the job page's sidebar (`rounded-xl`, no shadow, `sticky
top-20`). Since the goal is visual parity with the job page specifically, the sidebar
is hand-rolled with the same literal classes rather than adapted from `Card`, matching
how `JobView.svelte` itself does it.

**No `sticky` behavior debate:** the sidebar carries `sticky top-20` exactly like the
job page's, since the talent page's right column (header + up to four stacked
sections) can exceed one viewport height the same way a job's tab content can.

## Risks / Trade-offs

- **Widening the page from 768px to 1152px changes how every existing section reads**
  (role cards, chip rows) even though none of their markup changes — line lengths for
  wrapped chip rows will look sparser at the new width. Mitigation: this is the
  explicitly requested outcome (match jobview's sizing), and the existing sections use
  `flex-wrap` chip/card layouts that already tolerate a wider container without any
  code changes.
- **The empty-state check at line 166** (`!card.skills.length && !card.roles.length &&
  ...`) must keep including `card.skills` even after the Skills block moves out of the
  main flex column and into the sidebar — otherwise a candidate with only skills and no
  roles/education/certifications would incorrectly show the "hasn't published anything
  yet" message alongside their (now sidebar-rendered) skills. Mitigation: the check
  stays as-is; it does not need to move just because the Skills markup moves.

## Migration Plan

Pure front-end change, no data migration. Ships as an ordinary PR; nothing to
backfill, no feature flag needed since it fully replaces the previous layout for the
one route it touches.

Rollback: revert the single-file commit.
