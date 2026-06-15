<script lang="ts">
  import { Button } from '$lib/ui';

  const GITHUB = 'https://github.com/strelov1/freehire';
  const CLI = 'https://github.com/strelov1/freehire-cli';
  // Deep-link to the file a contributor edits to add their own source.
  const SOURCES = `${GITHUB}/blob/main/sources.yml`;

  // The real adapters behind the pipeline — listed verbatim so the page never
  // overpromises sources the ingest worker can't actually crawl.
  const sources = ['Greenhouse', 'Lever', 'Ashby', 'Teamtailor', 'SuccessFactors'];

  // Illustrative postings for the hero feed. Decorative, not live data — they
  // exist to show what "one clean feed" looks like, normalized into one shape.
  const feed = [
    { title: 'Senior Backend Engineer', company: 'Linear', tag: 'Remote', meta: 'Go · $180–220k' },
    { title: 'Staff Frontend Engineer', company: 'Stripe', tag: 'Remote', meta: 'TypeScript · $210–260k' },
    { title: 'Product Designer', company: 'Vercel', tag: 'Hybrid', meta: 'Figma · $150–190k' },
    { title: 'ML Engineer', company: 'Hugging Face', tag: 'Remote', meta: 'Python · $190–240k' },
    { title: 'Platform Engineer', company: 'Grafana', tag: 'Remote', meta: 'Kubernetes · $160–200k' },
    { title: 'Data Engineer', company: 'Datadog', tag: 'Hybrid', meta: 'Spark · $170–210k' },
  ];
  // Doubled so the vertical scroll loops seamlessly (see the styles below).
  const feedLoop = [...feed, ...feed];

  // Repeated to over-fill the row (the names are short) so the marquee never
  // shows a gap; the -50% loop stays seamless because the copies are identical.
  const sourcesMarquee = [...sources, ...sources, ...sources, ...sources];

  const steps = [
    {
      n: '01',
      title: 'Aggregate',
      body: 'Adapters crawl public ATS boards — Greenhouse, Lever, Ashby and more — straight from the companies that are hiring.',
    },
    {
      n: '02',
      title: 'Normalize & dedupe',
      body: 'Every posting is mapped to one schema and deduplicated, so the same role never shows up twice across boards.',
    },
    {
      n: '03',
      title: 'Enrich',
      body: 'An AI pass tags each job with stack, seniority, remote policy and salary — so the filters actually work.',
    },
  ];
</script>

<div class="landing">
  <!-- Hero. Left: the pitch. Right: a live-feeling feed of normalized postings
       that *is* the product. The dotted background fades out via a radial mask. -->
  <section class="grid-bg relative -mx-4 px-4 pb-16 pt-14 sm:pt-20">
    <div class="grid items-center gap-12 lg:grid-cols-[1.05fr_0.95fr]">
      <div>
        <p class="reveal font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground" style="--d:0ms">
          // open-source · free &amp; transparent
        </p>

        <h1
          class="reveal mt-6 max-w-2xl text-balance text-5xl font-semibold leading-[0.95] tracking-tighter sm:text-7xl"
          style="--d:80ms"
        >
          Every tech job,<br />in one clean feed.
        </h1>

        <p class="reveal mt-7 max-w-xl text-lg leading-relaxed text-muted-foreground" style="--d:160ms">
          FreeHire pulls openings straight from company career boards, strips the duplicates, and tags each
          one with stack, seniority and location — so you search jobs, not job boards.
        </p>

        <div class="reveal mt-9 flex flex-wrap items-center gap-3" style="--d:240ms">
          <Button href="/jobs" variant="primary" size="lg">Browse jobs</Button>
          <Button href={GITHUB} target="_blank" rel="noopener noreferrer" variant="outline" size="lg">
            View on GitHub ↗
          </Button>
        </div>
      </div>

      <!-- Vertical job feed. Hidden below lg to keep the mobile hero focused. -->
      <div class="reveal feed-viewport hidden h-[480px] lg:block" style="--d:320ms">
        <div class="feed-track">
          {#each feedLoop as job, i (i)}
            <article
              class="feed-card flex items-center gap-3 rounded-xl border border-border bg-card px-4 py-3 transition-colors"
              aria-hidden={i >= feed.length}
            >
              <div
                class="grid size-9 shrink-0 place-items-center rounded-lg border border-border font-mono text-sm font-medium"
              >
                {job.company.charAt(0)}
              </div>
              <div class="min-w-0 flex-1">
                <p class="truncate text-sm font-medium">{job.title}</p>
                <p class="truncate text-xs text-muted-foreground">{job.company}</p>
              </div>
              <div class="shrink-0 text-right">
                <p class="font-mono text-[11px] text-muted-foreground">{job.meta}</p>
                <span
                  class="mt-1 inline-block rounded-full border border-border px-2 py-0.5 text-[11px] font-medium text-muted-foreground"
                >
                  {job.tag}
                </span>
              </div>
            </article>
          {/each}
        </div>
      </div>
    </div>
  </section>

  <!-- Sourced-from marquee: the real adapters, scrolling. Honest list only. -->
  <section class="marquee-band flex items-center gap-6 overflow-hidden border-y border-border py-4">
    <span class="shrink-0 font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground">
      sourced from
    </span>
    <div class="marquee-viewport flex-1 overflow-hidden">
      <div class="marquee-track flex items-center gap-8 whitespace-nowrap">
        {#each sourcesMarquee as src, i (i)}
          <span class="text-sm text-muted-foreground">{src}</span>
          <span class="text-border" aria-hidden="true">·</span>
        {/each}
      </div>
    </div>
  </section>

  <!-- Mission. A single large statement carries this section. -->
  <section class="py-16 sm:py-20">
    <p class="font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground">// mission</p>
    <p class="mt-6 max-w-3xl text-2xl font-medium leading-snug tracking-tight sm:text-3xl">
      A job board that works for you, not for recruiters. No paywalls, no sponsored listings, no résumé
      harvesting. Free to use, and free to fork.
    </p>
  </section>

  <!-- How it works — three steps that mirror the actual ingest pipeline. -->
  <section class="border-t border-border py-16 sm:py-20">
    <p class="font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground">// how it works</p>
    <div class="mt-10 grid gap-px overflow-hidden rounded-xl border border-border bg-border sm:grid-cols-3">
      {#each steps as step (step.n)}
        <div class="group bg-background p-6 transition-colors hover:bg-secondary/40 sm:p-7">
          <span class="font-mono text-sm text-muted-foreground transition-colors group-hover:text-foreground">
            {step.n}
          </span>
          <h3 class="mt-4 text-lg font-semibold tracking-tight">{step.title}</h3>
          <p class="mt-2 text-sm leading-relaxed text-muted-foreground">{step.body}</p>
        </div>
      {/each}
    </div>
  </section>

  <!-- CLI / agents — the same API from the terminal. Mirrors the open-source
       section's two-column copy + terminal figure. -->
  <section class="border-t border-border py-16 sm:py-20">
    <p class="font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground">// for agents</p>
    <div class="mt-6 grid gap-10 lg:grid-cols-2 lg:items-center">
      <div>
        <h2 class="max-w-md text-3xl font-semibold tracking-tight sm:text-4xl">
          Use it from the terminal. Built for agents.
        </h2>
        <p class="mt-5 max-w-md leading-relaxed text-muted-foreground">
          freehire is also a CLI, so an AI agent or a script can search, open and track jobs over the
          same API — no browser. Create an API key and it lives in
          <code class="font-mono text-foreground">~/.freehire/creds.json</code>; add
          <code class="font-mono text-foreground">--json</code> for machine-readable output.
        </p>
        <div class="mt-8 flex flex-wrap gap-3">
          <Button href="/cli" variant="primary" size="lg">Explore the CLI</Button>
          <Button href={CLI} target="_blank" rel="noopener noreferrer" variant="ghost" size="lg">
            View on GitHub ↗
          </Button>
        </div>
      </div>

      <figure
        class="overflow-hidden rounded-xl border border-border bg-secondary/60 font-mono text-sm shadow-sm"
      >
        <figcaption
          class="flex items-center gap-2 border-b border-border px-4 py-2.5 text-xs text-muted-foreground"
        >
          <span class="size-2.5 rounded-full bg-muted-foreground/30"></span>
          terminal
        </figcaption>
        <pre class="overflow-x-auto p-4 leading-relaxed"><span class="text-muted-foreground"># install — no Go needed</span>
curl -fsSL <span class="text-foreground">https://freehire.dev/install.sh</span> | sh

<span class="text-muted-foreground"># authenticate once, then search &amp; track</span>
freehire auth login --token <span class="text-foreground">fhk_…</span>
freehire search <span class="text-foreground">"golang"</span> --remote --region eu
freehire save <span class="text-foreground">&lt;slug&gt;</span></pre>
      </figure>
    </div>
  </section>

  <!-- Open source / contribute. The sources.yml snippet is the concrete proof
       that "add a company" really is one line. -->
  <section class="border-t border-border py-16 sm:py-20">
    <p class="font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground">// open source</p>
    <div class="mt-6 grid gap-10 lg:grid-cols-2 lg:items-center">
      <div>
        <h2 class="max-w-md text-3xl font-semibold tracking-tight sm:text-4xl">
          Built in the open. Add your own source.
        </h2>
        <p class="mt-5 max-w-md leading-relaxed text-muted-foreground">
          Missing a company? Add it yourself — one entry in
          <code class="font-mono text-foreground">sources.yml</code>. A new ATS platform is one adapter. The
          backend is Go, the frontend is Svelte, the license is MIT — issues and pull requests welcome.
        </p>
        <div class="mt-8 flex flex-wrap gap-3">
          <Button href={SOURCES} target="_blank" rel="noopener noreferrer" variant="primary" size="lg">
            Add a source ↗
          </Button>
          <Button href={GITHUB} target="_blank" rel="noopener noreferrer" variant="ghost" size="lg">
            Star on GitHub ↗
          </Button>
        </div>
      </div>

      <figure
        class="overflow-hidden rounded-xl border border-border bg-secondary/60 font-mono text-sm shadow-sm"
      >
        <figcaption
          class="flex items-center gap-2 border-b border-border px-4 py-2.5 text-xs text-muted-foreground"
        >
          <span class="size-2.5 rounded-full bg-muted-foreground/30"></span>
          sources.yml
        </figcaption>
        <pre class="overflow-x-auto p-4 leading-relaxed"><span class="text-muted-foreground"># add a company in three lines</span>
- company: <span class="text-foreground">Acme</span>
  provider: <span class="text-foreground">greenhouse</span>
  board: <span class="text-foreground">acme</span></pre>
      </figure>
    </div>
  </section>
</div>

<style>
  /* Dotted background, faded toward the edges with a radial mask so it frames
     the hero without competing with the copy. --muted-foreground at low opacity
     keeps the dots visible in both light and dark. */
  .grid-bg::before {
    content: '';
    position: absolute;
    inset: 0;
    z-index: -1;
    background-image: radial-gradient(var(--muted-foreground) 1px, transparent 1.2px);
    background-size: 22px 22px;
    opacity: 0.16;
    -webkit-mask-image: radial-gradient(ellipse 90% 75% at 25% 0%, #000 18%, transparent 80%);
    mask-image: radial-gradient(ellipse 90% 75% at 25% 0%, #000 18%, transparent 80%);
  }

  /* Vertical job feed: a doubled list scrolling up forever. The loop is seamless
     because each card carries its own bottom margin, so 2N cards split exactly in
     half at translateY(-50%). Top/bottom are faded with a mask (not overlays, so
     it never blocks the hover-to-pause). */
  .feed-viewport {
    -webkit-mask-image: linear-gradient(to bottom, transparent, #000 12%, #000 86%, transparent);
    mask-image: linear-gradient(to bottom, transparent, #000 12%, #000 86%, transparent);
  }
  .feed-track {
    animation: feed-scroll 34s linear infinite;
  }
  .feed-card {
    margin-bottom: 0.75rem;
  }
  .feed-card:hover {
    border-color: color-mix(in oklch, var(--foreground) 25%, transparent);
  }
  .feed-viewport:hover .feed-track {
    animation-play-state: paused;
  }
  @keyframes feed-scroll {
    to {
      transform: translateY(-50%);
    }
  }

  /* Horizontal source marquee: list quadrupled, slides one half-width then loops. */
  .marquee-viewport {
    -webkit-mask-image: linear-gradient(to right, transparent, #000 6%, #000 94%, transparent);
    mask-image: linear-gradient(to right, transparent, #000 6%, #000 94%, transparent);
  }
  .marquee-track {
    width: max-content;
    animation: marquee 30s linear infinite;
  }
  .marquee-band:hover .marquee-track {
    animation-play-state: paused;
  }
  @keyframes marquee {
    to {
      transform: translateX(-50%);
    }
  }

  /* One orchestrated page-load: each .reveal rises in, staggered by its --d. */
  .reveal {
    opacity: 0;
    animation: rise 0.7s cubic-bezier(0.2, 0.7, 0.2, 1) forwards;
    animation-delay: var(--d, 0ms);
  }
  @keyframes rise {
    from {
      opacity: 0;
      transform: translateY(10px);
    }
    to {
      opacity: 1;
      transform: none;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .reveal,
    .feed-track,
    .marquee-track {
      animation: none;
    }
    .reveal {
      opacity: 1;
    }
  }
</style>
