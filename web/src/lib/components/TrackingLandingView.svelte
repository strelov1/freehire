<script lang="ts">
  import { Clock, Mail } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { Badge, Button, EntityLogo, SectionLabel } from '$lib/ui';
  import HomeFunnel from '$lib/components/HomeFunnel.svelte';
  import { TRACKING_FAQ } from '$lib/trackingFaq';

  interface PreviewCard {
    company: string;
    title: string;
    badge?: string;
    silenceDays?: number;
    mailCount?: number;
    hasNotes?: boolean;
  }

  interface PreviewColumn {
    id: string;
    label: string;
    count: number;
    cards: PreviewCard[];
  }

  // Illustrative "My jobs" board for the hero figure — decorative, not live data,
  // and deliberately not anyone's real search: mirrors the product's own stage
  // vocabulary (STAGE_GROUPS) and BoardCard's signal set (stage badge, silence
  // day-counter, linked-mail count, notes marker) so the preview never shows a
  // signal the real board doesn't have. Column `count` can exceed its visible
  // cards, same as the real board once a column scrolls.
  const board: PreviewColumn[] = [
    {
      id: 'preparing',
      label: 'Preparing',
      count: 6,
      cards: [
        { company: 'Grafana', title: 'Platform Engineer' },
        { company: 'Vercel', title: 'Staff Frontend Engineer', hasNotes: true },
      ],
    },
    {
      id: 'applied',
      label: 'Applied',
      count: 18,
      cards: [
        { company: 'Stripe', title: 'Senior Backend Engineer', badge: 'Applied', mailCount: 1 },
        { company: 'Datadog', title: 'Data Engineer', badge: 'Screening', hasNotes: true },
        { company: 'Hugging Face', title: 'ML Engineer', badge: 'Applied' },
      ],
    },
    {
      id: 'interview',
      label: 'Interview',
      count: 2,
      cards: [
        { company: 'Linear', title: 'Senior Backend Engineer', silenceDays: 12, mailCount: 3 },
        { company: 'Figma', title: 'Product Designer', hasNotes: true },
      ],
    },
    {
      id: 'offer',
      label: 'Offer',
      count: 1,
      cards: [{ company: 'Notion', title: 'Staff Frontend Engineer', mailCount: 2 }],
    },
  ];

  // Illustrative pipeline snapshot for the funnel below — the counts match the
  // hero board above (same 64 applications, split the same way) so the two
  // figures read as one search rather than two disconnected mockups.
  const funnel = {
    applications: 64,
    buckets: { preparing: 6, applied: 18, interview: 2, offer: 1, closed: 37 },
  };

  const signals = [
    {
      key: 'silence',
      title: 'A day-counter when it goes quiet',
      body: 'Once an application has gone unanswered past the point worth noticing, its card carries a day-counter — days since the last contact — so a stalled thread stays visible instead of aging quietly at the bottom of a column.',
    },
    {
      key: 'inbox',
      title: 'Replies attach themselves',
      body: 'Connect your mail and a recruiter reply is tagged with what it says, attached to the application it belongs to, and walks the card forward for you.',
      href: resolve('/features/inbox'),
      cta: 'How the inbox works',
    },
    {
      key: 'notes',
      title: 'A note only you see',
      body: "A contact's name, a red flag, the reason you applied — attach a private note to any card and it travels with it wherever it moves.",
    },
    {
      key: 'search',
      title: 'Search the board itself',
      body: 'Once it is long, filter by company or role right on the board — the same search that narrows the list and calendar views.',
    },
  ];

  const views = [
    { key: 'board', title: 'Board', body: 'Drag a card through its stages by hand.' },
    { key: 'list', title: 'List', body: 'Every application, one row each, sorted and filtered.' },
    { key: 'pipeline', title: 'Pipeline', body: 'The funnel below — the shape of the whole search.' },
    { key: 'calendar', title: 'Calendar', body: 'Anything with a date, an interview included.' },
  ];
</script>

<div class="flex flex-col">
  <!-- Hero. Headline and pitch, then the board itself, full width — a kanban
       needs the room a side-column hero figure doesn't have. -->
  <section class="dot-grid -mx-4 px-4 pb-16 pt-8">
    <SectionLabel text="job tracking" />
    <h1 class="mt-6 max-w-2xl text-balance text-4xl font-semibold leading-[1.0] tracking-tighter sm:text-6xl">
      Every application, one board. Nothing falls through.
    </h1>
    <p class="mt-7 max-w-xl text-lg leading-relaxed text-muted-foreground">
      Save what's worth a second look and freehire keeps it visible until it's resolved — through
      four stages, from Preparing to Offer, with a day-counter when an employer goes quiet and a
      place for the notes only you need.
    </p>
    <div class="mt-9 flex flex-wrap items-center gap-3">
      <Button href={resolve('/my/tracking')} variant="primary" size="lg">Open Tracking</Button>
      <Button href={resolve('/cli')} variant="outline" size="lg">Track from the CLI</Button>
    </div>

    <!-- Board preview: the real kanban columns, hairline-separated. -->
    <figure class="mt-12 overflow-hidden rounded-xl border border-border bg-card shadow-sm">
      <figcaption class="flex items-center gap-2 border-b border-border px-4 py-2.5 text-xs text-muted-foreground">
        <span class="size-2.5 rounded-full bg-muted-foreground/30"></span>
        My jobs · Board
      </figcaption>
      <div class="grid gap-px bg-border sm:grid-cols-4">
        {#each board as col (col.id)}
          <div class="bg-background p-4">
            <div class="flex items-center justify-between">
              <span class="text-xs font-medium uppercase tracking-wide text-muted-foreground">{col.label}</span>
              <span class="font-mono text-[11px] text-muted-foreground">{col.count}</span>
            </div>
            <div class="mt-3 flex flex-col gap-2">
              {#each col.cards as card (card.title)}
                <article class="flex flex-col gap-1.5 rounded-lg border border-border bg-card p-3 shadow-sm">
                  <span class="flex items-center gap-1.5 text-sm font-semibold">
                    <EntityLogo name={card.company} shape="square" size="xs" />
                    <span class="min-w-0 truncate">{card.company}</span>
                  </span>
                  <span class="line-clamp-2 text-sm">{card.title}</span>
                  <span class="flex flex-wrap items-center gap-x-1.5 gap-y-1">
                    {#if card.badge}
                      <Badge variant="secondary">{card.badge}</Badge>
                    {/if}
                    {#if card.silenceDays}
                      <span class="flex items-center gap-0.5 text-xs tabular-nums text-warning-strong">
                        <Clock class="size-3 shrink-0" aria-hidden="true" />
                        {card.silenceDays}d
                      </span>
                    {/if}
                    {#if card.mailCount}
                      <span class="flex items-center gap-0.5 text-xs tabular-nums text-muted-foreground">
                        <Mail class="size-3 shrink-0" aria-hidden="true" />
                        {card.mailCount}
                      </span>
                    {/if}
                    {#if card.hasNotes}
                      <svg
                        class="size-3 shrink-0 text-muted-foreground"
                        viewBox="0 0 24 24"
                        fill="none"
                        stroke="currentColor"
                        stroke-width="2"
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        aria-hidden="true"
                      >
                        <path d="M8 7h8M8 12h8M8 17h5" />
                      </svg>
                    {/if}
                  </span>
                </article>
              {/each}
            </div>
          </div>
        {/each}
      </div>
    </figure>
  </section>

  <!-- What a card carries. -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="every card, at a glance" />
    <div class="mt-6 max-w-2xl">
      <h2 class="text-3xl font-semibold tracking-tight sm:text-4xl">The card tells you where things stand.</h2>
      <p class="mt-5 leading-relaxed text-muted-foreground">
        Nothing here is a control you have to open a panel to find. It's read straight off the card.
      </p>
    </div>
    <div class="mt-10 grid gap-px overflow-hidden rounded-xl border border-border bg-border sm:grid-cols-2">
      {#each signals as s (s.key)}
        <div class="bg-background p-6 sm:p-7">
          <h3 class="text-lg font-semibold tracking-tight">{s.title}</h3>
          <p class="mt-2 text-sm leading-relaxed text-muted-foreground">{s.body}</p>
          {#if s.href}
            <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- internal route already passed through resolve() when building `signals`; the linter can't trace it via the variable -->
            <a href={s.href} class="mt-4 inline-block text-sm font-medium text-foreground underline-offset-4 hover:underline">
              {s.cta} →
            </a>
          {/if}
        </div>
      {/each}
    </div>
  </section>

  <!-- Nudges — trimmed to a pointer at the dedicated Notifications page, which
       covers this alongside job alerts and shows what the emails look like. -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="nudges" />
    <div class="mt-6 max-w-2xl">
      <h2 class="text-3xl font-semibold tracking-tight sm:text-4xl">
        It follows up before you have to remember to.
      </h2>
      <p class="mt-5 leading-relaxed text-muted-foreground">
        The same silence a card's day-counter shows also goes out as a nudge — over email,
        Telegram or push — so a stalled application doesn't just sit there. See what that looks
        like on the <a href={resolve('/features/notifications')} class="font-medium text-foreground underline-offset-4 hover:underline">Notifications</a> page.
      </p>
      <div class="mt-8 flex flex-wrap gap-3">
        <Button href={resolve('/features/notifications')} variant="primary" size="lg">See how notifications work</Button>
        <Button href={resolve('/my/notifications/settings')} variant="ghost" size="lg">Notification settings</Button>
      </div>
    </div>
  </section>

  <!-- Your pipeline — the funnel view of the same board. -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="your pipeline" />
    <div class="mt-6 max-w-2xl">
      <h2 class="text-3xl font-semibold tracking-tight sm:text-4xl">See where every application lands.</h2>
      <p class="mt-5 leading-relaxed text-muted-foreground">
        As cards move through the board, freehire rolls them into one funnel — how many are still in
        Preparing, sitting in Applied, in an active Interview loop, or turned into an Offer. Whatever
        settled moves to Closed: out of the active board, never erased.
      </p>
    </div>
    <figure class="mt-10 overflow-hidden rounded-xl border border-border bg-card shadow-sm">
      <figcaption class="flex items-center gap-2 border-b border-border px-4 py-2.5 text-xs text-muted-foreground">
        <span class="size-2.5 rounded-full bg-muted-foreground/30"></span>
        My jobs · Pipeline
      </figcaption>
      <div class="p-5 sm:p-8">
        <HomeFunnel applications={funnel.applications} buckets={funnel.buckets} />
      </div>
    </figure>
  </section>

  <!-- Four views over the same data. -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="four ways to look at it" />
    <div class="mt-6 max-w-2xl">
      <h2 class="text-3xl font-semibold tracking-tight sm:text-4xl">Board, List, Pipeline, Calendar.</h2>
      <p class="mt-5 leading-relaxed text-muted-foreground">
        Same applications, four tabs. Switch when the board isn't the shape you need right now.
      </p>
    </div>
    <div class="mt-10 grid gap-px overflow-hidden rounded-xl border border-border bg-border sm:grid-cols-2 lg:grid-cols-4">
      {#each views as v (v.key)}
        <div class="bg-background p-6">
          <h3 class="text-sm font-semibold tracking-tight">{v.title}</h3>
          <p class="mt-2 text-sm leading-relaxed text-muted-foreground">{v.body}</p>
        </div>
      {/each}
    </div>
  </section>

  <!-- Terminal / agents. -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="from the terminal" />
    <div class="mt-6 grid gap-10 lg:grid-cols-2 lg:items-center">
      <div>
        <h2 class="max-w-md text-3xl font-semibold tracking-tight sm:text-4xl">
          Or let a script keep the board current.
        </h2>
        <p class="mt-5 max-w-md leading-relaxed text-muted-foreground">
          The freehire CLI drives the same board with one API key — save, apply, move a stage, leave a
          note — so your own agent can keep it current without a browser.
        </p>
        <div class="mt-8 flex flex-wrap gap-3">
          <Button href={resolve('/cli')} variant="primary" size="lg">Explore the CLI</Button>
          <Button href={resolve('/docs/api')} variant="ghost" size="lg">API reference</Button>
        </div>
      </div>
      <figure class="overflow-hidden rounded-xl border border-border bg-secondary/60 font-mono text-sm shadow-sm">
        <figcaption class="flex items-center gap-2 border-b border-border px-4 py-2.5 text-xs text-muted-foreground">
          <span class="size-2.5 rounded-full bg-muted-foreground/30"></span>
          terminal
        </figcaption>
        <pre class="overflow-x-auto p-4 leading-relaxed"><span class="text-muted-foreground"># save or apply — either starts tracking it</span>
freehire <span class="text-foreground">apply &lt;slug&gt;</span>
freehire <span class="text-foreground">save &lt;slug&gt;</span>

<span class="text-muted-foreground"># move it yourself, or let the inbox do it</span>
freehire <span class="text-foreground">stage &lt;slug&gt; --to interview</span>

<span class="text-muted-foreground"># a private note, attached to the card</span>
freehire <span class="text-foreground">note &lt;slug&gt; "referred by a former teammate"</span></pre>
      </figure>
    </div>
  </section>

  <!-- FAQ. Visible answers and the FAQPage JSON-LD share TRACKING_FAQ. -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="faq" />
    <h2 class="mt-6 max-w-md text-3xl font-semibold tracking-tight sm:text-4xl">Frequently asked questions.</h2>
    <dl class="mt-10 grid gap-px overflow-hidden rounded-xl border border-border bg-border sm:grid-cols-2">
      {#each TRACKING_FAQ as item (item.question)}
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
      <h2 class="text-2xl font-semibold tracking-tight">Stop losing track of where you applied.</h2>
      <p class="max-w-xl leading-relaxed text-muted-foreground">
        Save the next job worth a second look and it's on the board — Preparing, Applied, Interview,
        Offer, tracked from there on.
      </p>
      <div class="flex flex-wrap gap-3">
        <Button href={resolve('/jobs')} variant="primary" size="lg">Browse jobs</Button>
        <Button href={resolve('/my/tracking')} variant="outline" size="lg">Open Tracking</Button>
      </div>
    </div>
  </section>
</div>
