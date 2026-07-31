# Icons

One icon set: **`@lucide/svelte`**. An inline `<svg>` in `web/` needs a reason, and
there are only two good ones — it is a brand mark, or it is a chart.

Brand marks: `BrandMark.svelte` (and its twin at `web/static/favicon.svg`),
`ProviderIcon.svelte` (five OAuth provider logos), `CompanyLogo.svelte`'s fallback.

Charts, which are drawings rather than icons: `ActivityBars`, `GrowthArea`,
`RateDonut`, `MatchAnalysisFull`, `HomeFunnel`, `PipelineFunnel`.

`NoteEditor.svelte` is the standing exception: EasyMDE takes toolbar glyphs as HTML
strings, not Svelte components, so its seven are Lucide paths inlined by hand. The
constraint is real; leave them.

## Open consolidations

Verified against `main` on 2026-07-31. Each is a cleanup nobody has picked up, not
a defect.

1. **`PipelineFunnel.svelte` (94 lines) and `HomeFunnel.svelte` (107)** are the same
   Sankey funnel twice — same geometry constants, same ribbon-path formula.
   `HomeFunnel`'s own header says *"Pipeline feature is not yet on main. Seam: when
   it lands, the two can fold into one shared presentational component."* It has
   landed. A shared component taking `buckets: {key,label,color}[]` collapses both.
2. **Four inline UI glyphs with direct Lucide equivalents already used elsewhere** —
   `BoardCard.svelte`'s "has notes" (`StickyNote`), `DocsNav.svelte`'s search
   (`Search`), and `docs/api/+layout.svelte`'s hamburger and chevron (`Menu`,
   `ChevronDown`).
3. **`BrandMark.svelte` and `web/static/favicon.svg` carry byte-identical path
   data.** The static file is forced — browsers fetch `/favicon.svg` — but the path
   string could live once in `web/src/lib/brandPath.ts`.
