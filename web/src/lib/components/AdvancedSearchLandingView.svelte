<script lang="ts">
  import { resolve } from '$app/paths';
  import { UserRound } from '@lucide/svelte';
  import { Button, Chip, NumberedGrid, SectionLabel } from '$lib/ui';
  import { pillClass, pillTitle } from './facets/pill';
  import { ADVANCED_SEARCH_FAQ } from '$lib/advancedSearchFaq';

  // The hero demo is a real, working miniature of the filter panel — the same
  // pillClass/pillTitle helpers PillGroup.svelte uses — not a screenshot. Values
  // are drawn from real facets (CATEGORY_LABELS, REGIONS) so the claim it makes
  // ("this is what the filters look like") stays true; the selection itself is
  // just a plausible illustrative starting point, not live search state.
  type PillState = 'off' | 'include' | 'exclude';
  type DemoRow = { label: string; options: { value: string; label: string }[] };

  // Shared by both demos below: the real pill click cycle (PillGroup.svelte / pill.ts).
  const CYCLE: PillState[] = ['off', 'include', 'exclude'];
  const NEXT_STATE: Record<PillState, PillState> = { off: 'include', include: 'exclude', exclude: 'off' };

  const demoRows: DemoRow[] = [
    {
      label: 'Specialization',
      options: [
        { value: 'backend', label: 'Backend' },
        { value: 'frontend', label: 'Frontend' },
        { value: 'devops', label: 'DevOps' },
        { value: 'sre', label: 'SRE' },
      ],
    },
    {
      label: 'Region',
      options: [
        { value: 'eu', label: 'Europe' },
        { value: 'apac', label: 'APAC' },
        { value: 'latam', label: 'LATAM' },
        { value: 'mena', label: 'MENA' },
      ],
    },
    {
      label: 'Skills',
      options: [
        { value: 'go', label: 'Go' },
        { value: 'php', label: 'PHP' },
        { value: 'rust', label: 'Rust' },
        { value: 'kubernetes', label: 'Kubernetes' },
      ],
    },
  ];

  let demoState = $state<Record<string, PillState>>({
    backend: 'include',
    eu: 'include',
    apac: 'include',
    go: 'include',
    php: 'exclude',
  });

  function cycleDemo(value: string) {
    demoState[value] = NEXT_STATE[demoState[value] ?? 'off'];
  }

  const demoIncluded = $derived(Object.values(demoState).filter((s) => s === 'include').length);
  const demoExcluded = $derived(Object.values(demoState).filter((s) => s === 'exclude').length);

  // The "include, exclude, or leave alone" section's own single-pill demo: one
  // clickable pill instead of three static ones, paired with a state-flow legend
  // that highlights the step it's currently on — the flow itself is the diagram,
  // not three unconnected examples floating in space.
  let exampleState = $state<PillState>('off');
  function cycleExample() {
    exampleState = NEXT_STATE[exampleState];
  }
  const STEP_LABEL: Record<PillState, string> = { off: 'Off', include: 'Include', exclude: 'Exclude' };

  // The 20 facets in web/src/lib/facets.ts' FACETS array, grouped for a reader
  // rather than listed flat — each group is a real cluster of param names, not
  // an arbitrary split. Chips are real values from CATEGORY_LABELS, REGIONS,
  // DOMAIN_LABELS, RELOCATION_LABELS, ENGLISH_LEVEL_LABELS, and the reality facet.
  const facetGroups = [
    {
      title: 'Role & skill',
      params: 'role · category · seniority · skills · ai_archetype',
      body: 'Search by title, the broader discipline it sits in, seniority band, tech stack, or one of six AI-specific skill signatures.',
      chips: ['Backend', 'Senior', 'Go', 'RAG Application Builder'],
    },
    {
      title: 'Where',
      params: 'regions · countries · cities · work_mode · relocation',
      body: 'A macro region, a specific country or city, remote versus hybrid versus on-site, and whether the company relocates people at all.',
      chips: ['Remote', 'Europe', 'Berlin', 'Relocation supported'],
    },
    {
      title: 'Company',
      params: 'company_type · company_slug · domains',
      body: 'One named employer, or the shape of the company — in-house team versus agency, and the industry it sells into.',
      chips: ['In-house', 'FinTech', 'DevTools'],
    },
    {
      title: 'Terms',
      params: 'employment_type · salary_currency · english_level',
      body: 'Full-time or part-time, the currency a salary is quoted in, and the English level a posting actually asks for.',
      chips: ['Full-time', 'USD', 'B2'],
    },
    {
      title: 'Trust & language',
      params: 'reality · posting_language · source',
      body: 'Postings that look freshly filled versus ones that read as evergreen or stale, the language a listing is written in, and which board it came from.',
      chips: ['Fresh', 'Likely evergreen', 'English'],
    },
    {
      title: 'Curated',
      params: 'collections',
      body: "Editorial and credential-based lists that don't map to one field — remote-worldwide roles, YC companies, verified certifications.",
      chips: ['Remote Worldwide', 'YC-backed'],
    },
  ];

  const saveSteps = [
    {
      n: '01',
      title: 'Save this search',
      body: 'One click turns your current filters into a saved search, listed under My filters right in the panel. Save more than one and switch between them — no separate setup screen.',
    },
    {
      n: '02',
      title: 'Pick a channel',
      body: 'Telegram, email, or push to your phone. Turn on as many as you want, per saved search.',
    },
    {
      n: '03',
      title: 'Get matches, not a feed',
      body: "When a new job matches, freehire sends it the moment it's indexed. Nothing to refresh, nothing to remember to check.",
    },
  ];

  const cliCommands = [
    { cmd: 'facets', desc: "Every filter's live values and counts — the vocabulary to filter by." },
    { cmd: 'search "golang" --remote --region eu', desc: 'List matching jobs from a terminal or a script.' },
  ];
</script>

<div class="flex flex-col">
  <!-- Hero. Left: the pitch, front-loading breadth, exclude and save — the three
       things this page has to land even for someone who reads only the first
       screen. Right: the real pill mechanics, shrunk down and clickable. -->
  <section class="dot-grid -mx-4 grid items-center gap-12 px-4 pb-16 pt-8 lg:grid-cols-[1.05fr_0.95fr]">
    <div>
      <SectionLabel text="advanced search" />
      <h1 class="mt-6 max-w-2xl text-balance text-4xl font-semibold leading-[1.0] tracking-tighter sm:text-6xl">
        Cut the list down to the jobs you'd actually take.
      </h1>
      <p class="mt-7 max-w-xl text-lg leading-relaxed text-muted-foreground">
        Role, seniority, stack, region, company type, salary currency, even whether a posting looks
        real — twenty filters, and almost every one can also mean <span class="text-foreground"
          >"not this."</span
        > Build the search once, save it to your profile, and let freehire keep running it for you.
      </p>
      <div class="mt-9 flex flex-wrap items-center gap-3">
        <Button href={resolve('/jobs')} variant="primary" size="lg">Browse jobs</Button>
        <Button href={resolve('/my/notifications/searches')} variant="outline" size="lg">
          Saved searches &amp; alerts
        </Button>
      </div>
    </div>

    <!-- Live mini filter panel: the real pillClass/pillTitle states, clickable. -->
    <figure class="overflow-hidden rounded-xl border border-border bg-card shadow-sm">
      <figcaption class="flex items-center gap-2 border-b border-border px-4 py-2.5 text-xs text-muted-foreground">
        <span class="size-2.5 rounded-full bg-muted-foreground/30"></span>
        freehire · Filters
      </figcaption>
      <div class="flex flex-col gap-4 p-4 sm:p-5">
        {#each demoRows as row (row.label)}
          <div>
            <p class="mb-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">{row.label}</p>
            <div class="flex flex-wrap gap-2">
              {#each row.options as opt (opt.value)}
                {@const state = demoState[opt.value] ?? 'off'}
                <button
                  type="button"
                  onclick={() => cycleDemo(opt.value)}
                  title={pillTitle(state === 'include', state === 'exclude', true)}
                  class={pillClass(
                    state !== 'off',
                    state === 'exclude',
                    'px-3 py-1.5 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50',
                  )}
                >
                  {opt.label}
                </button>
              {/each}
            </div>
          </div>
        {/each}
      </div>
      <div
        class="flex items-center justify-between border-t border-border px-4 py-2.5 text-xs text-muted-foreground sm:px-5"
      >
        <span>Click a pill — off → include → exclude → off</span>
        <span class="font-mono tabular-nums">{demoIncluded} in · {demoExcluded} out</span>
      </div>
    </figure>
  </section>

  <!-- The 20 facets, grouped. -->
  <section class="border-t border-border py-14 sm:py-16">
    <SectionLabel text="what you can filter by" />
    <p class="mt-6 max-w-2xl text-sm leading-relaxed text-muted-foreground">
      Every group below is a real cluster of filter panel sections — the mono line under each
      title is the exact parameter name the filter panel, the API and the CLI all use.
    </p>
    <div
      class="mt-8 grid gap-px overflow-hidden rounded-xl border border-border bg-border sm:grid-cols-2 lg:grid-cols-3"
    >
      {#each facetGroups as group (group.title)}
        <div class="flex flex-col gap-3 bg-background p-6 sm:p-7">
          <div>
            <h3 class="text-lg font-semibold tracking-tight">{group.title}</h3>
            <p class="mt-1 font-mono text-xs text-muted-foreground">{group.params}</p>
          </div>
          <p class="text-sm leading-relaxed text-muted-foreground">{group.body}</p>
          <div class="mt-auto flex flex-wrap gap-1.5 pt-1">
            {#each group.chips as chip (chip)}
              <Chip>{chip}</Chip>
            {/each}
          </div>
        </div>
      {/each}
    </div>
  </section>

  <!-- Include / exclude, explained via the same pill states. -->
  <section class="border-t border-border py-14 sm:py-16">
    <SectionLabel text="include, exclude, or leave alone" />
    <div class="mt-6 grid gap-10 lg:grid-cols-[1.05fr_0.95fr] lg:items-center">
      <p class="max-w-xl text-sm leading-relaxed text-muted-foreground">
        Nineteen of the twenty filters cycle through three states on a single click: off, then
        include, then exclude, then back to off. Exclude isn't a separate control to go find — it's
        the same pill, one more click. Rule out a source you don't trust, a stack you're done with,
        or a company you already applied to, without leaving the field empty — which would only mean
        "don't care."
      </p>
      <div class="flex flex-col items-center gap-5 rounded-xl border border-border bg-card p-6 sm:p-8">
        <button
          type="button"
          onclick={cycleExample}
          title={pillTitle(exampleState === 'include', exampleState === 'exclude', true)}
          class={pillClass(
            exampleState !== 'off',
            exampleState === 'exclude',
            'px-6 py-3 text-base focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50',
          )}
        >
          Go
        </button>
        <div class="flex items-center gap-2 font-mono text-xs">
          {#each CYCLE as step, i (step)}
            {#if i > 0}<span class="text-muted-foreground/50">→</span>{/if}
            <span class={step === exampleState ? 'font-semibold text-foreground' : 'text-muted-foreground'}>
              {STEP_LABEL[step]}
            </span>
          {/each}
          <span class="text-muted-foreground/50">→</span>
          <span class="text-muted-foreground">Off</span>
        </div>
        <p class="text-center text-xs text-muted-foreground">Click the pill — same three states as the panel above.</p>
      </div>
    </div>
  </section>

  <!-- Apply my profile: the reverse direction — profile feeds filters, not the
       other way around — and a real, concrete exclude example (avoided skills). -->
  <section class="border-t border-border py-14 sm:py-16">
    <SectionLabel text="start from your profile" />
    <div class="mt-6 grid gap-10 lg:grid-cols-[1.05fr_0.95fr] lg:items-center">
      <div>
        <h2 class="max-w-md text-3xl font-semibold tracking-tight sm:text-4xl">
          Or skip the setup — apply my profile.
        </h2>
        <p class="mt-5 max-w-xl text-sm leading-relaxed text-muted-foreground">
          Everything you filled in becomes a starting set of filters, one click, right in the
          panel: your specialization and skills go in as includes, the skills you're avoiding go
          in as excludes, and your work-location preferences — remote regions, on-site countries
          and cities, whether you'd relocate — come along too. Nothing to re-type.
        </p>
      </div>
      <div class="rounded-xl border border-border bg-card p-6 sm:p-8">
        <div class="flex items-center gap-2 text-sm font-medium">
          <span class="flex size-8 items-center justify-center rounded-full bg-secondary">
            <UserRound class="size-4" aria-hidden="true" />
          </span>
          Apply my profile
        </div>
        <dl class="mt-5 space-y-3 text-sm">
          <div class="flex items-baseline justify-between gap-4">
            <dt class="text-muted-foreground">Specialization &amp; skills</dt>
            <dd class="font-mono text-xs text-muted-foreground">→ include</dd>
          </div>
          <div class="flex items-baseline justify-between gap-4">
            <dt class="text-muted-foreground">Skills you're avoiding</dt>
            <dd class="font-mono text-xs text-muted-foreground">→ exclude</dd>
          </div>
          <div class="flex items-baseline justify-between gap-4">
            <dt class="text-muted-foreground">Where you'll work</dt>
            <dd class="font-mono text-xs text-muted-foreground">→ region, country, city</dd>
          </div>
        </dl>
        <Button href={resolve('/my/profile')} variant="outline" size="sm" class="mt-6">
          Fill in your profile
        </Button>
      </div>
    </div>
  </section>

  <!-- Save to profile, as the genuine 3-step sequence it is. -->
  <section class="border-t border-border py-14 sm:py-16">
    <SectionLabel text="save it once" />
    <h2 class="mt-6 max-w-md text-3xl font-semibold tracking-tight sm:text-4xl">
      Set the filters up once. They'll keep working after you close the tab.
    </h2>
    <NumberedGrid items={saveSteps} class="mt-10 sm:grid-cols-3" />
    <div class="mt-8 flex flex-wrap gap-3">
      <Button href={resolve('/jobs')} variant="primary" size="lg">Start filtering</Button>
      <Button href={resolve('/my/notifications/searches')} variant="ghost" size="lg">
        Saved searches &amp; alerts
      </Button>
    </div>
  </section>

  <!-- CLI — modest: names the shared vocabulary, links out to the full /cli page
       rather than duplicating its command reference. -->
  <section class="border-t border-border py-14 sm:py-16">
    <SectionLabel text="from the terminal" />
    <div class="mt-6 grid gap-10 lg:grid-cols-[1fr_1fr]">
      <p class="max-w-xl text-sm leading-relaxed text-muted-foreground">
        Every parameter name on this page — <code class="font-mono text-foreground">role</code>,
        <code class="font-mono text-foreground">regions</code>,
        <code class="font-mono text-foreground">skills</code>,
        <code class="font-mono text-foreground">company_type</code> — is the same one the public API
        and the freehire CLI use. Start from the live vocabulary, then filter from a script instead
        of a browser tab.
      </p>
      <div class="rounded-lg border border-border bg-secondary/40 p-4">
        <dl class="space-y-3">
          {#each cliCommands as row (row.cmd)}
            <div>
              <dt class="font-mono text-sm">
                <span class="text-muted-foreground">freehire</span> {row.cmd}
              </dt>
              <dd class="text-sm leading-relaxed text-muted-foreground">{row.desc}</dd>
            </div>
          {/each}
        </dl>
        <Button href={resolve('/cli')} variant="outline" size="sm" class="mt-4">
          Read the CLI docs →
        </Button>
      </div>
    </div>
  </section>

  <!-- FAQ. The visible answers and the FAQPage JSON-LD share ADVANCED_SEARCH_FAQ. -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="faq" />
    <h2 class="mt-6 max-w-md text-3xl font-semibold tracking-tight sm:text-4xl">
      Frequently asked questions.
    </h2>
    <dl class="mt-10 grid gap-px overflow-hidden rounded-xl border border-border bg-border sm:grid-cols-2">
      {#each ADVANCED_SEARCH_FAQ as item (item.question)}
        <div class="bg-background p-6 sm:p-7">
          <dt class="text-lg font-semibold tracking-tight">{item.question}</dt>
          <dd class="mt-2 text-sm leading-relaxed text-muted-foreground">{item.answer}</dd>
        </div>
      {/each}
    </dl>
  </section>

  <!-- Closing CTA. -->
  <section class="border-t border-border py-16 sm:py-20">
    <div class="flex flex-col items-start gap-4 rounded-xl border border-border bg-secondary/40 p-6 sm:p-8">
      <h2 class="text-2xl font-semibold tracking-tight">Stop scrolling past the same noise.</h2>
      <p class="max-w-xl leading-relaxed text-muted-foreground">
        Filter down to what you'd actually apply to, save it, and let freehire watch for the rest.
      </p>
      <div class="flex flex-wrap gap-3">
        <Button href={resolve('/jobs')} variant="primary" size="lg">Browse jobs</Button>
        <Button href={resolve('/my/notifications/searches')} variant="outline" size="lg">
          Saved searches &amp; alerts
        </Button>
      </div>
    </div>
  </section>
</div>
