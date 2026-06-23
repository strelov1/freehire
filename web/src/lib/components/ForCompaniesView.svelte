<script lang="ts">
  import { resolve } from '$app/paths';
  import { Button } from '$lib/ui';

  const repoUrl = 'https://github.com/strelov1/freehire';
  const telegramUrl = 'https://t.me/freehiredev';

  // Self-serve ATS platforms: the multi-tenant boards where a company maps to a
  // single board entry, so listing is one line in sources/<provider>.yml. This is
  // a curated subset of the source registry on purpose — aggregators (RemoteOK)
  // and single-company adapters (Uber) aren't something a company can self-add.
  const atsPlatforms = [
    'Greenhouse',
    'Lever',
    'Ashby',
    'Workable',
    'Recruitee',
    'SmartRecruiters',
    'Personio',
    'BambooHR',
    'Workday',
    'Teamtailor',
    'Rippling',
  ];

  // How a listed board stays in sync. Honest — these are the actual ingest
  // mechanics (scheduled crawl, normalize + dedup, stale-sweep close), not
  // marketing promises.
  const freshness = [
    {
      title: 'Crawled regularly',
      body: 'Your board is polled on a schedule — about once a day. New roles show up in the catalogue and search within a few hours of being published.',
    },
    {
      title: 'Normalized & deduplicated',
      body: 'Postings are parsed into one shared schema, enriched and deduplicated against every other source, so a role that appears twice is shown once.',
    },
    {
      title: 'Auto-closes when you remove a role',
      body: 'Take a vacancy down and it closes on its own once it stops appearing in your board — no action needed on your side. Your ATS stays the source of truth.',
    },
  ];

  const benefits = [
    {
      title: 'Free & open-source',
      body: 'freehire is a non-commercial aggregator. Listing your board costs nothing — no fees, no paywall, no upsell.',
    },
    {
      title: 'A developer audience',
      body: 'Your roles land in one normalized feed alongside Greenhouse, Lever and Ashby boards — searchable by stack, seniority and location.',
    },
    {
      title: 'You stay in control',
      body: 'There is no second copy to maintain. We read your ATS; what you publish there is exactly what gets listed, and what you remove gets closed.',
    },
  ];
</script>

<div class="flex flex-col gap-16">
  <!-- Hero -->
  <section class="flex flex-col gap-7">
    <p class="font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground">
      // for companies &amp; employers
    </p>
    <h1 class="max-w-2xl text-balance text-4xl font-semibold leading-[1.0] tracking-tighter sm:text-6xl">
      Get your whole job board indexed.
    </h1>
    <p class="max-w-xl text-lg leading-relaxed text-muted-foreground">
      Want your openings here? If you use a supported ATS, add your board with one line — or
      contribute an adapter for your own. freehire crawls it regularly and folds the roles into one
      clean, searchable feed of tech jobs.
    </p>
    <div class="flex flex-wrap items-center gap-3">
      <Button href={repoUrl} variant="primary" size="lg">Add your board on GitHub</Button>
      <Button href={resolve('/companies')} variant="outline" size="lg">Browse companies</Button>
    </div>
  </section>

  <!-- Two paths -->
  <section class="flex flex-col gap-6">
    <h2 class="text-2xl font-semibold tracking-tight">Two ways to get listed</h2>
    <div class="grid gap-6 lg:grid-cols-2">
      <!-- Path A: supported ATS -->
      <div class="flex flex-col gap-4 rounded-lg border border-border p-5">
        <div class="flex flex-col gap-1">
          <span class="font-mono text-sm text-muted-foreground">01</span>
          <h3 class="text-base font-semibold tracking-tight">You already use a supported ATS</h3>
        </div>
        <p class="text-sm leading-relaxed text-muted-foreground">
          Open an issue or pull request adding one entry to the matching board file under
          <code class="font-mono text-foreground">sources/</code>. That's the whole change:
        </p>
        <pre
          class="overflow-x-auto rounded-lg border border-border bg-secondary/60 p-3 font-mono text-sm leading-relaxed"><span
            class="text-muted-foreground"># sources/greenhouse.yml</span>
- company: Acme Corp
  board: acmecorp</pre>
        <div class="flex flex-wrap gap-2">
          {#each atsPlatforms as ats (ats)}
            <span
              class="rounded-md border border-border bg-secondary/40 px-2 py-0.5 font-mono text-xs text-muted-foreground"
            >
              {ats}
            </span>
          {/each}
        </div>
      </div>

      <!-- Path B: unsupported ATS -->
      <div class="flex flex-col gap-4 rounded-lg border border-border p-5">
        <div class="flex flex-col gap-1">
          <span class="font-mono text-sm text-muted-foreground">02</span>
          <h3 class="text-base font-semibold tracking-tight">Your ATS isn't on the list yet</h3>
        </div>
        <p class="text-sm leading-relaxed text-muted-foreground">
          freehire is open source, so a new platform is one adapter in
          <code class="font-mono text-foreground">internal/sources</code> — a small reader over your
          public job API. Send it as a pull request, or open an issue asking for the platform and
          point us at your careers page.
        </p>
        <div class="flex flex-wrap gap-3">
          <Button href={repoUrl} variant="outline" size="sm">View the source</Button>
          <Button href="{repoUrl}/issues/new" variant="ghost" size="sm">Request a platform</Button>
        </div>
      </div>
    </div>
  </section>

  <!-- Publish via the API -->
  <section class="flex flex-col gap-6">
    <h2 class="text-2xl font-semibold tracking-tight">Or publish via the API</h2>
    <div class="grid gap-6 lg:grid-cols-2 lg:items-start">
      <div class="flex flex-col gap-4">
        <p class="text-sm leading-relaxed text-muted-foreground">
          Rather push than be pulled? Mint an
          <a href={resolve('/my/api-keys')} class="font-medium text-foreground underline-offset-4 hover:underline"
            >API key</a
          >
          and post roles straight to freehire — one call per vacancy, from your own systems or the
          <a href={resolve('/cli')} class="font-medium text-foreground underline-offset-4 hover:underline"
            >freehire CLI</a
          >. Submissions go through the same moderator review as everything else, then join the
          catalogue.
        </p>
        <p class="text-sm leading-relaxed text-muted-foreground">
          See the
          <a href={resolve('/docs/api')} class="font-medium text-foreground underline-offset-4 hover:underline"
            >API reference</a
          >
          for the full payload and endpoints.
        </p>
        <div class="flex flex-wrap gap-3">
          <Button href={resolve('/my/api-keys')} variant="outline" size="sm">Get an API key</Button>
          <Button href={resolve('/docs/api')} variant="ghost" size="sm">API reference</Button>
        </div>
      </div>
      <pre
        class="overflow-x-auto rounded-lg border border-border bg-secondary/60 p-3 font-mono text-sm leading-relaxed"><span
          class="text-muted-foreground"># POST a vacancy — goes to moderation review</span>
curl -X POST https://freehire.dev/api/v1/submissions \
  -H <span class="text-foreground">"Authorization: Bearer $FREEHIRE_API_KEY"</span> \
  -H <span class="text-foreground">"Content-Type: application/json"</span> \
  -d '&#123;"url": "https://acme.com/careers/go",
       "title": "Senior Go Engineer",
       "company": "Acme Corp"&#125;'</pre>
    </div>
  </section>

  <!-- How it stays fresh -->
  <section class="flex flex-col gap-6">
    <h2 class="text-2xl font-semibold tracking-tight">How it stays fresh</h2>
    <div class="grid gap-6 sm:grid-cols-3">
      {#each freshness as f (f.title)}
        <div class="flex flex-col gap-2">
          <h3 class="text-base font-semibold tracking-tight">{f.title}</h3>
          <p class="text-sm leading-relaxed text-muted-foreground">{f.body}</p>
        </div>
      {/each}
    </div>
  </section>

  <!-- Why -->
  <section class="grid gap-6 sm:grid-cols-3">
    {#each benefits as b (b.title)}
      <div class="flex flex-col gap-2 rounded-lg border border-border p-5">
        <h2 class="text-base font-semibold tracking-tight">{b.title}</h2>
        <p class="text-sm leading-relaxed text-muted-foreground">{b.body}</p>
      </div>
    {/each}
  </section>

  <!-- Closing CTA -->
  <section class="flex flex-col items-start gap-4 rounded-lg border border-border bg-secondary/40 p-6">
    <h2 class="text-xl font-semibold tracking-tight">Ready to list your board?</h2>
    <p class="max-w-xl text-sm leading-relaxed text-muted-foreground">
      Open an issue on GitHub and we'll take it from there. Questions first? Reach the community on
      Telegram.
    </p>
    <div class="flex flex-wrap gap-3">
      <Button href="{repoUrl}/issues/new" variant="primary" size="lg">Open an issue on GitHub</Button>
      <Button href={telegramUrl} variant="outline" size="lg">Join on Telegram</Button>
    </div>
  </section>
</div>
