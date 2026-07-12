<script lang="ts">
  // The About page's values block — the promises behind the mission, shown only
  // on /about (HomeView carries the mission and renders on / too, so these live
  // in a standalone component to keep them off the landing page).
  const values = [
    {
      n: '01',
      title: 'For candidates, not recruiters.',
      body: "When interests collide, the candidate wins — every time. No sponsored listings, no pay-to-rank, no selling your data to whoever pays. If a choice is good for us but bad for the person job-hunting, we don't make it.",
    },
    {
      n: '02',
      title: 'The whole market, not a slice.',
      body: "We pull from as many sources as we can — company boards, ATS platforms, niche feeds — so you see the full set of options, not a hand-picked subset. Nothing is hidden because it didn't pay to be shown.",
    },
    {
      n: '03',
      title: 'From board to you, in minutes.',
      body: "Fast everywhere it touches you — a site that responds instantly, and new postings that land in your feed while they're still fresh. We measure speed from the company's board all the way to the notification in your pocket.",
    },
    {
      n: '04',
      title: 'Open, so you can check.',
      body: "freehire is open source and free to fork. You don't have to take our word that there's no catch — the code is right there. Transparency is how we keep the other promises honest.",
    },
    {
      n: '05',
      title: 'Built to last, not to flip.',
      body: "We're not chasing an exit or a data sale. freehire is built to keep serving candidates for years — each improvement compounds instead of being a one-off, so it gets better the longer it runs.",
    },
  ];
</script>

<section class="border-t border-border py-16 sm:py-20">
  <p class="font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground">// values</p>
  <dl class="mt-10 divide-y divide-border border-y border-border">
    {#each values as v (v.n)}
      <div class="grid gap-2 py-6 sm:grid-cols-[auto_1fr] sm:gap-8">
        <span class="font-mono text-sm text-muted-foreground">{v.n}</span>
        <div>
          <dt class="text-lg font-semibold tracking-tight">{v.title}</dt>
          <dd class="mt-2 max-w-3xl text-sm leading-relaxed text-muted-foreground">{v.body}</dd>
        </div>
      </div>
    {/each}
  </dl>
</section>
