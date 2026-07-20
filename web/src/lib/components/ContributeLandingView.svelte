<script lang="ts">
  import { resolve } from '$app/paths';
  import { Button } from '$lib/ui';

  const repoUrl = 'https://github.com/strelov1/freehire';
  const telegramUrl = 'https://t.me/freehiredev';

  // A representative slice of the ~37 multi-tenant ATS whose company board we can read
  // straight from a pasted link. Not the full list on purpose — enough to be concrete.
  const supportedAts = [
    'Greenhouse',
    'Lever',
    'Ashby',
    'Workable',
    'Recruitee',
    'SmartRecruiters',
    'BambooHR',
    'Personio',
    'PeopleForce',
    'Gupy',
    'Freshteam',
    'JazzHR',
  ];

  // The three-step flow, kept honest — this is exactly what the backend does with a
  // pasted link (recognize the board from the URL, dedup against the catalogue, reward
  // a genuinely new board).
  const steps = [
    {
      n: '01',
      title: 'Paste a link',
      body: 'A single vacancy or a company’s careers page — on your contributions page, or sent to your linked Telegram chat. No forms, no fields.',
    },
    {
      n: '02',
      title: 'We read the board from the URL',
      body: 'From the link alone we recognize the ATS and the company board — instantly, without fetching anything.',
    },
    {
      n: '03',
      title: 'New board? You’re rewarded',
      body: 'If it’s a company we don’t track yet — the role isn’t in our catalogue — we add the board, crawl all of its openings, and credit you 1 AI credit.',
    },
  ];
</script>

<div class="flex flex-col gap-16">
  <!-- Hero -->
  <section class="dot-grid -mx-4 flex flex-col gap-7 px-4 pb-4 pt-2">
    <p class="font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground">
      // contribute
    </p>
    <h1 class="max-w-3xl text-balance text-4xl font-semibold leading-[1.0] tracking-tighter sm:text-6xl">
      A search engine is only as good as its coverage.
    </h1>
    <p class="max-w-xl text-lg leading-relaxed text-muted-foreground">
      freehire is a free, open-source search engine for tech jobs. Its one job is to be
      <span class="text-foreground">complete</span> — to surface every open role on the market, in
      one searchable place. A company we don’t track is a blind spot. Help us close them.
    </p>
    <div class="flex flex-wrap items-center gap-3">
      <Button href={resolve('/my/contributions')} variant="primary" size="lg">Contribute a board</Button>
      <Button href={resolve('/companies')} variant="outline" size="lg">Browse companies</Button>
    </div>
  </section>

  <!-- Why coverage -->
  <section class="flex flex-col gap-6">
    <h2 class="text-2xl font-semibold tracking-tight">Coverage is the whole game</h2>
    <p class="max-w-2xl text-sm leading-relaxed text-muted-foreground">
      Every source we crawl folds into one normalized, deduplicated feed — searchable by stack,
      seniority and location. The more company boards we track, the more of the real market you can
      actually see and compare. Openness is how it scales: because freehire is open source, anyone
      can widen that coverage.
    </p>
  </section>

  <!-- Two ways -->
  <section class="flex flex-col gap-6">
    <h2 class="text-2xl font-semibold tracking-tight">Two ways to widen it</h2>
    <div class="grid gap-6 lg:grid-cols-2">
      <!-- Deep path -->
      <div class="flex flex-col gap-4 rounded-lg border border-border p-5">
        <div class="flex flex-col gap-1">
          <span class="font-mono text-sm text-muted-foreground">The open way</span>
          <h3 class="text-base font-semibold tracking-tight">Add a board slug, or a whole adapter</h3>
        </div>
        <p class="text-sm leading-relaxed text-muted-foreground">
          The source is on GitHub. Add one line to a board file to track a company on a supported
          ATS, or write a small adapter for a platform we don’t read yet. See
          <a href={resolve('/for-companies')} class="font-medium text-foreground underline-offset-4 hover:underline">for companies</a>
          for the details.
        </p>
        <div class="flex flex-wrap gap-3">
          <Button href={repoUrl} variant="outline" size="sm">View the source</Button>
          <Button href="{repoUrl}/issues/new" variant="ghost" size="sm">Request a platform</Button>
        </div>
      </div>

      <!-- Easy path -->
      <div class="flex flex-col gap-4 rounded-lg border border-border bg-secondary/40 p-5">
        <div class="flex flex-col gap-1">
          <span class="font-mono text-sm text-muted-foreground">The easy way</span>
          <h3 class="text-base font-semibold tracking-tight">Just paste a link — and get rewarded</h3>
        </div>
        <p class="text-sm leading-relaxed text-muted-foreground">
          We simplified that first step down to a single link, and we reward the community for it.
          Spot a company we’re missing? Drop its link and you’ve just made the market a little
          more complete for everyone — and earned an AI credit for it.
        </p>
        <div class="flex flex-wrap gap-2">
          {#each supportedAts as ats (ats)}
            <span
              class="rounded-md border border-border bg-background/60 px-2 py-0.5 font-mono text-xs text-muted-foreground"
            >
              {ats}
            </span>
          {/each}
          <span class="rounded-md px-2 py-0.5 font-mono text-xs text-muted-foreground">+ more</span>
        </div>
      </div>
    </div>
  </section>

  <!-- How it works -->
  <section class="flex flex-col gap-6">
    <h2 class="text-2xl font-semibold tracking-tight">How contributing works</h2>
    <div class="grid gap-6 sm:grid-cols-3">
      {#each steps as s (s.n)}
        <div class="flex flex-col gap-2">
          <span class="font-mono text-sm text-muted-foreground">{s.n}</span>
          <h3 class="text-base font-semibold tracking-tight">{s.title}</h3>
          <p class="text-sm leading-relaxed text-muted-foreground">{s.body}</p>
        </div>
      {/each}
    </div>
    <p class="max-w-2xl text-sm leading-relaxed text-muted-foreground">
      The unit is the <span class="text-foreground">company board</span>, not a single vacancy —
      once we know the board, we pull in all of its jobs. So a second link to a company we already
      cover earns nothing, and that’s deliberate: you’re rewarded for genuinely new coverage.
    </p>
  </section>

  <!-- What credits are for -->
  <section class="flex flex-col gap-4 rounded-lg border border-border p-6">
    <h2 class="text-xl font-semibold tracking-tight">What your credits are for</h2>
    <p class="max-w-2xl text-sm leading-relaxed text-muted-foreground">
      AI credits are the thank-you for widening the market — and they fund your own search: spend
      them to check how your CV matches the live market and to get it sharper — the coverage you help
      build, working back for you.
    </p>
  </section>

  <!-- Closing CTA -->
  <section class="flex flex-col items-start gap-4 rounded-lg border border-border bg-secondary/40 p-6">
    <h2 class="text-xl font-semibold tracking-tight">Found a company we’re missing?</h2>
    <p class="max-w-xl text-sm leading-relaxed text-muted-foreground">
      Paste its link — a vacancy or its careers page. If we don’t track it yet, you’ve found
      new coverage, and the credit is yours.
    </p>
    <div class="flex flex-wrap gap-3">
      <Button href={resolve('/my/contributions')} variant="primary" size="lg">Contribute a board</Button>
      <Button href={telegramUrl} variant="outline" size="lg">Join on Telegram</Button>
    </div>
  </section>
</div>
