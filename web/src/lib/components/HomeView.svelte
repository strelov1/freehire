<script lang="ts">
  import { resolve } from '$app/paths';
  import { Button } from '$lib/ui';
  import HomeFunnel from '$lib/components/HomeFunnel.svelte';
  import { NumberedGrid, SectionLabel } from '$lib/ui';
  import { HOME_FAQ } from '$lib/homeFaq';

  // Live catalogue figures from the page's server load, all from one snapshot. Any may
  // be null when the API is unreachable, so each has a static fallback (see `figures`).
  const {
    stats,
  }: { stats: { jobs: number | null; companies: number | null; sources: number | null } } = $props();

  // Compact display for the live figures (2,939,967 → "2.9M+"). The fallbacks are
  // rounded DOWN from the last measured totals: the catalogue does not only grow
  // (a `cmd/prune` sweep shrinks it), so a fallback rounded up would eventually
  // overstate a count the API can no longer back.
  const nf = new Intl.NumberFormat('en', { notation: 'compact', maximumFractionDigits: 1 });
  const compact = (n: number | null, fallback: string) => (n == null ? fallback : `${nf.format(n)}+`);

  // The under-the-fold stats strip: three live figures plus the licensing constant.
  //
  // "sources" counts every registered adapter, not just ATS platforms. It was labelled
  // "ATS platforms" while carrying the whole registry — and of today's 227 adapters, 104
  // are aggregators and 30 are single-company career feeds, so the label was counting
  // things it did not describe. It is live now, so it also stops going stale.
  const figures = $derived([
    { value: compact(stats.jobs, '3.3M+'), label: 'open jobs' },
    { value: compact(stats.companies, '290K+'), label: 'companies' },
    { value: stats.sources == null ? '220+' : String(stats.sources), label: 'sources' },
    { value: '100%', label: 'open source' },
  ]);

  // "Straight from the source" — the value prop that separates freehire from an
  // aggregator. Mirrors the README "Why freehire?" points.
  const sourced = [
    {
      n: '01',
      title: 'Straight from the source',
      body: "Every listing is crawled from a company's own ATS and links to the original posting — no recruiter reposts, no aggregator middlemen.",
    },
    {
      n: '02',
      title: 'One schema, deduplicated',
      body: 'The same role posted to three boards collapses into one entry — normalized into a single shape and deduped on a stable key.',
    },
    {
      n: '03',
      title: 'No dead links',
      body: 'A liveness sweep probes open postings and closes the ones that 404 or expire, so what you open is still open.',
    },
  ];

  const GITHUB = 'https://github.com/strelov1/freehire';
  const CLI = 'https://github.com/strelov1/freehire-cli';
  // Where a contributor opens a request to have a new source added.
  const ISSUES = `${GITHUB}/issues`;

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

  // Illustrative "My jobs" board — decorative, not live data. Mirrors the real
  // kanban (board.ts BOARD_COLUMNS) so the preview never promises a flow the
  // product doesn't have: drag a saved job along its stages to the offer.
  const board = [
    { label: 'Saved', cards: [{ title: 'Data Engineer', company: 'Datadog' }] },
    { label: 'Applied', cards: [{ title: 'Staff Frontend Engineer', company: 'Stripe' }] },
    { label: 'Interview', cards: [{ title: 'Senior Backend Engineer', company: 'Linear' }] },
    { label: 'Offer', cards: [{ title: 'Platform Engineer', company: 'Grafana' }] },
  ];

  // Illustrative application-pipeline snapshot for the funnel below — decorative,
  // not live data. Counts sum to `applications`, and the keys are the product's own
  // pipeline groups, so the preview never promises a flow the product doesn't have.
  const funnel = {
    applications: 42,
    buckets: {
      applied: 17,
      interview: 9,
      offer: 4,
      closed: 12,
    },
  };

  // The feature landings, each of which explains one surface in depth. Kept here
  // rather than only in the footer so the story on / and /about has a doorway.
  const features = [
    {
      href: resolve('/features/inbox'),
      title: 'Inbox',
      body: 'Connect your mail and every recruiter reply is tagged with what it says, attached to the application it belongs to, and walked forward on your board.',
      cta: 'How the inbox works',
    },
    {
      href: resolve('/features/tailor'),
      title: 'CV tailoring',
      body: 'Reframe your CV toward one vacancy — pulling forward the experience you already have but buried, and asking about anything your history does not support.',
      cta: 'How tailoring works',
    },
    {
      href: resolve('/features/referrals'),
      title: 'Referrals',
      body: 'Ask an employee inside the company to put your name forward — anonymously and for free — instead of applying cold.',
      cta: 'How referrals work',
    },
    {
      href: resolve('/features/ghost-jobs'),
      title: 'Ghost jobs',
      body: 'Some postings are not being filled at all. freehire marks the ones whose behaviour and applicant outcomes say so, and shows you every fact behind the warning.',
      cta: 'How the signal works',
    },
  ];

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
        <SectionLabel text="open-source · free &amp; transparent" class="reveal" style="--d:0ms" />

        <h1
          class="reveal mt-6 max-w-2xl text-balance text-5xl font-semibold leading-[0.95] tracking-tighter sm:text-7xl"
          style="--d:80ms"
        >
          Every tech job,<br />one search.
        </h1>

        <p class="reveal mt-7 max-w-xl text-lg leading-relaxed text-muted-foreground" style="--d:160ms">
          Freehire pulls openings straight from company career boards, strips the duplicates, and tags each
          one with stack, seniority and location — so you search jobs, not job boards.
        </p>

        <div class="reveal mt-9 flex flex-wrap items-center gap-3" style="--d:240ms">
          <Button href={resolve('/')} variant="primary" size="lg">Browse jobs</Button>
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

  <!-- Catalogue scale, right under the fold: two live totals (server-loaded, with
       a static "+" fallback) and two constants. The first proof of the hero's claim. -->
  <section class="py-10 sm:py-12">
    <SectionLabel text="the catalogue" />
    <dl class="mt-6 grid grid-cols-2 gap-px overflow-hidden rounded-xl border border-border bg-border sm:grid-cols-4">
      {#each figures as f (f.label)}
        <div class="bg-background p-5 sm:p-6">
          <dt class="font-mono text-xs uppercase tracking-wide text-muted-foreground">{f.label}</dt>
          <dd class="mt-2 text-3xl font-semibold tracking-tight tabular-nums sm:text-4xl">{f.value}</dd>
        </div>
      {/each}
    </dl>
  </section>

  <!-- Straight from the source — what separates freehire from an aggregator. -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="straight from the source" />
    <NumberedGrid items={sourced} class="mt-10 sm:grid-cols-3" />
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
    <SectionLabel text="mission" />
    <p class="mt-6 max-w-3xl text-2xl font-medium leading-snug tracking-tight sm:text-3xl">
      A job board that works for you, not for recruiters. No paywalls, no sponsored listings, no CV
      harvesting. Free to use, and free to fork.
    </p>
  </section>

  <!-- How it works — three steps that mirror the actual ingest pipeline. -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="how it works" />
    <NumberedGrid items={steps} class="mt-10 sm:grid-cols-3" />
  </section>

  <!-- Track your search — the per-user My jobs board. A polished, hero-style
       preview of the real kanban; the board data is illustrative, not live. -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="track your search" />
    <div class="mt-6 max-w-2xl">
      <h2 class="text-3xl font-semibold tracking-tight sm:text-4xl">
        Track every application on your board.
      </h2>
      <p class="mt-5 leading-relaxed text-muted-foreground">
        Save the openings worth a second look, mark the ones you applied to, and drag each card through
        its stages — Saved → Applied → Interview → Offer. Your
        <code class="font-mono text-foreground">My jobs</code> board keeps the whole search in one place.
        Prefer the terminal? The <a href={resolve('/cli')} class="font-medium text-foreground underline-offset-4 hover:underline">freehire CLI</a>
        does the same — <code class="font-mono text-foreground">apply</code>,
        <code class="font-mono text-foreground">save</code>,
        <code class="font-mono text-foreground">stage</code> and
        <code class="font-mono text-foreground">note</code> any job from a script or an agent.
      </p>
      <p class="mt-4 leading-relaxed text-muted-foreground">
        You don't have to move the cards by hand either: connect your mail and the
        <a href={resolve('/features/inbox')} class="font-medium text-foreground underline-offset-4 hover:underline">freehire inbox</a>
        tags each recruiter reply, attaches it to the application it belongs to, and walks the card
        forward for you.
      </p>
      <div class="mt-8 flex flex-wrap gap-3">
        <Button href={resolve('/my/tracking')} variant="primary" size="lg">Open Tracking</Button>
        <Button href={resolve('/cli')} variant="ghost" size="lg">Track from the CLI</Button>
      </div>
    </div>

    <!-- Board preview: the real kanban columns, hairline-separated (gap-px on a
         bg-border grid) like the "how it works" tiles. Stacks on mobile. -->
    <figure
      class="mt-10 overflow-hidden rounded-xl border border-border bg-card shadow-sm"
    >
      <figcaption
        class="flex items-center gap-2 border-b border-border px-4 py-2.5 text-xs text-muted-foreground"
      >
        <span class="size-2.5 rounded-full bg-muted-foreground/30"></span>
        My jobs · Board
      </figcaption>
      <div class="grid gap-px bg-border sm:grid-cols-4">
        {#each board as col (col.label)}
          <div class="bg-background p-4">
            <div class="flex items-center justify-between">
              <span class="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                {col.label}
              </span>
              <span class="font-mono text-[11px] text-muted-foreground">{col.cards.length}</span>
            </div>
            <div class="mt-3 flex flex-col gap-2">
              {#each col.cards as card (card.title)}
                <article class="flex items-center gap-3 rounded-lg border border-border bg-card px-3 py-2.5">
                  <div
                    class="grid size-8 shrink-0 place-items-center rounded-lg border border-border font-mono text-xs font-medium"
                  >
                    {card.company.charAt(0)}
                  </div>
                  <div class="min-w-0 flex-1">
                    <p class="truncate text-sm font-medium">{card.title}</p>
                    <p class="truncate text-xs text-muted-foreground">{card.company}</p>
                  </div>
                </article>
              {/each}
            </div>
          </div>
        {/each}
      </div>
    </figure>
  </section>

  <!-- Your pipeline — the analytical view of the same My jobs board: a static
       Sankey snapshot of where applications land. The counts are illustrative,
       not live data (see `funnel` above). -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="your pipeline" />
    <div class="mt-6 max-w-2xl">
      <h2 class="text-3xl font-semibold tracking-tight sm:text-4xl">
        See where every application lands.
      </h2>
      <p class="mt-5 leading-relaxed text-muted-foreground">
        As jobs move through your board, freehire rolls them into a pipeline snapshot — how many
        applications are still waiting on an answer, in progress, interviewing, or turned into an offer.
        One glance shows what's working and where things stall.
      </p>
    </div>

    <!-- Funnel figure, full width: the SVG's wide viewBox (W ≫ height) keeps the
         diagram at a sensible height even spanning the whole section. -->
    <figure class="mt-10 overflow-hidden rounded-xl border border-border bg-card shadow-sm">
      <figcaption
        class="flex items-center gap-2 border-b border-border px-4 py-2.5 text-xs text-muted-foreground"
      >
        <span class="size-2.5 rounded-full bg-muted-foreground/30"></span>
        My jobs · Pipeline
      </figcaption>
      <div class="p-5 sm:p-8">
        <HomeFunnel applications={funnel.applications} buckets={funnel.buckets} />
      </div>
    </figure>
  </section>

  <!-- The feature landings. Each has a page of its own; this is the doorway. -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="features" />
    <div class="mt-10 grid gap-px overflow-hidden rounded-xl border border-border bg-border sm:grid-cols-2">
      {#each features as f (f.href)}
        <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- internal route already passed through resolve() when building `features`; the linter can't trace it via the variable -->
        <a href={f.href} class="group bg-background p-6 transition-colors hover:bg-secondary/40 sm:p-7">
          <h3 class="text-lg font-semibold tracking-tight">{f.title}</h3>
          <p class="mt-2 text-sm leading-relaxed text-muted-foreground">{f.body}</p>
          <span class="mt-4 inline-block text-sm font-medium text-foreground underline-offset-4 group-hover:underline">
            {f.cta} →
          </span>
        </a>
      {/each}
    </div>
  </section>

  <!-- Notifications — save a filter, get matching jobs pushed to Telegram. -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="stay in the loop" />
    <div class="mt-6 max-w-2xl">
      <h2 class="text-3xl font-semibold tracking-tight sm:text-4xl">New jobs, straight to Telegram.</h2>
      <p class="mt-5 leading-relaxed text-muted-foreground">
        Save a search — your stack, seniority, region, salary — and subscribe to it. When a matching job is
        added, freehire sends it to you on Telegram as a tidy digest. No inbox clutter, no checking back:
        connect once from
        <a href={resolve('/my/notifications/searches')} class="font-medium text-foreground underline-offset-4 hover:underline">Saved searches &amp; alerts</a>
        and the openings come to you.
      </p>
      <div class="mt-8 flex flex-wrap gap-3">
        <Button href={resolve('/')} variant="primary" size="lg">Find &amp; save a filter</Button>
        <Button href={resolve('/my/notifications/searches')} variant="ghost" size="lg">Saved searches &amp; alerts</Button>
      </div>
    </div>
  </section>

  <!-- CLI / agents — the same API from the terminal. Mirrors the open-source
       section's two-column copy + terminal figure. -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="for agents" />
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
          <Button href={resolve('/cli')} variant="primary" size="lg">Explore the CLI</Button>
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
curl -fsSL <span class="text-foreground">https://freehire.me/install.sh</span> | sh

<span class="text-muted-foreground"># authenticate once, then search &amp; track</span>
freehire auth login --token <span class="text-foreground">fhk_…</span>
freehire search <span class="text-foreground">"golang"</span> --remote --region eu
freehire save <span class="text-foreground">&lt;slug&gt;</span></pre>
      </figure>
    </div>
  </section>

  <!-- Open source / contribute. -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="open source" />
    <div class="mt-6">
      <h2 class="max-w-md text-3xl font-semibold tracking-tight sm:text-4xl">
        Built in the open. Add your own source.
      </h2>
      <p class="mt-5 max-w-md leading-relaxed text-muted-foreground">
        Missing a company or source? Open an issue and we'll add it. A new ATS platform is one adapter. The
        backend is Go, the frontend is Svelte, the license is MIT — issues and pull requests welcome.
      </p>
      <div class="mt-8 flex flex-wrap gap-3">
        <Button href={ISSUES} target="_blank" rel="noopener noreferrer" variant="primary" size="lg">
          Add a source ↗
        </Button>
        <Button href={GITHUB} target="_blank" rel="noopener noreferrer" variant="ghost" size="lg">
          Star on GitHub ↗
        </Button>
      </div>
    </div>
  </section>

  <!-- FAQ. Visible answers mirror the FAQPage JSON-LD (see homeFaq.ts) so the
       structured data always matches what's on the page. -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="faq" />
    <h2 class="mt-6 max-w-md text-3xl font-semibold tracking-tight sm:text-4xl">
      Frequently asked questions.
    </h2>
    <dl class="mt-10 grid gap-px overflow-hidden rounded-xl border border-border bg-border sm:grid-cols-2">
      {#each HOME_FAQ as item (item.question)}
        <div class="bg-background p-6 sm:p-7">
          <dt class="text-lg font-semibold tracking-tight">{item.question}</dt>
          <dd class="mt-2 text-sm leading-relaxed text-muted-foreground">{item.answer}</dd>
        </div>
      {/each}
    </dl>
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
