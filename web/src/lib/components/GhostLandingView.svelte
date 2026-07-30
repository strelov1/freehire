<script lang="ts">
  import { resolve } from '$app/paths';
  import { Check, Minus } from '@lucide/svelte';
  import { Button } from '$lib/ui';
  import NumberedGrid from '$lib/components/NumberedGrid.svelte';
  import SectionLabel from '$lib/components/SectionLabel.svelte';
  import { GHOST_FAQ } from '$lib/ghostFaq';
  import { CONVERGENCE, GHOST_SIGNALS, WITNESS_GATE } from '$lib/ghostSignals';

  // The signals render from the same array the checklist reads, so this page cannot
  // fall behind the product — ghostSignals.test.ts fails if a criterion joins the
  // vocabulary without an explanation. See hire-features-landing-space.
  const structural = $derived(GHOST_SIGNALS.filter((s) => s.tier === 'structural'));
  const outcome = $derived(GHOST_SIGNALS.filter((s) => s.tier === 'outcome'));

  // Previews are MARKUP, never screenshots: a screenshot is light-theme, carries a
  // real employer's name, and freezes a UI that changes. These are the real
  // components' shapes, built from illustrative data.
  const demoChecklist = [
    {
      label: 'Posting behaves as evergreen',
      detail: 'Open 240 days · reposted 13× · 7 copies open at once',
      fired: true,
    },
    {
      label: "Not on the company's own careers board",
      detail: 'Checked 2 days ago',
      fired: true,
    },
    { label: 'Applications here went unanswered', detail: 'No data', fired: false },
    { label: 'People reported no response', detail: 'No data', fired: false },
  ];

  const levels = [
    {
      chip: 'Possibly inactive',
      scale: '2/4',
      tone: 'muted' as const,
      when: `${CONVERGENCE} criteria fired, but nobody who applied has told us anything`,
    },
    {
      chip: 'Likely inactive',
      scale: '3/4',
      tone: 'warn' as const,
      when: `at least ${WITNESS_GATE} different people who applied went unanswered, and something else fired too`,
    },
  ];

  const toneClass = {
    warn: 'border-amber-500/40 bg-amber-500/10 text-amber-700 dark:text-amber-400',
    muted: 'border-border text-muted-foreground',
  };
</script>

<article class="flex flex-col gap-14">
  <!-- Hero. The number does the arguing; the promise is deliberately small. -->
  <header class="flex flex-col gap-5">
    <SectionLabel text="ghost jobs" />
    <h1 class="max-w-3xl text-3xl font-semibold tracking-tight sm:text-4xl">
      Some postings are not being filled. freehire tells you which, and why it thinks so.
    </h1>
    <p class="max-w-2xl text-base leading-relaxed text-muted-foreground">
      Between 18% and 27% of listings on the big boards are jobs nobody is working to fill —
      kept up to collect CVs, to look like growth, or because nobody took them down. You cannot
      tell from the page. Every hour spent on one is unpaid work you will never hear about.
    </p>
    <p class="max-w-2xl text-base leading-relaxed">
      freehire watches two things it can actually check: how a posting behaves over time, and
      what happened to people who applied. When enough of them line up, the job carries a mark
      —&nbsp;and the facts behind it.
    </p>
    <div class="flex flex-wrap gap-3">
      <Button href={resolve('/jobs')} variant="primary" size="lg">Browse jobs</Button>
      <Button href={resolve('/my/tracking')} variant="ghost" size="lg">Track your applications</Button>
    </div>
  </header>

  <!-- What it will not say. Placed before the mechanics on purpose: the limit is the
       feature's most important property, not a disclaimer at the bottom. -->
  <section class="flex flex-col gap-4">
    <SectionLabel text="the line we do not cross" />
    <div class="grid gap-4 sm:grid-cols-2">
      <div class="rounded-lg border border-border p-4">
        <p class="mb-1.5 text-sm font-medium">What we can check</p>
        <p class="text-sm leading-relaxed text-muted-foreground">
          “Open 240 days, reposted 13 times, seven copies live at once, and absent from the
          employer's own careers board.” Every part of that is verifiable, and every part is
          shown to you.
        </p>
      </div>
      <div class="rounded-lg border border-dashed border-border p-4">
        <p class="mb-1.5 text-sm font-medium text-muted-foreground">What we never claim</p>
        <p class="text-sm leading-relaxed text-muted-foreground">
          “This company is not really hiring.” That is a claim about somebody's intent, which
          no amount of data here can observe. So the wording stays hedged, and the word
          <em>ghost</em> never appears on a job.
        </p>
      </div>
    </div>
  </section>

  <!-- The four signals, rendered from the product's own vocabulary. -->
  <section class="flex flex-col gap-6">
    <SectionLabel text="the four signals" />
    <p class="max-w-2xl text-sm leading-relaxed text-muted-foreground">
      Two describe the <strong class="font-medium text-foreground">posting</strong>. Two describe
      what happened to a <strong class="font-medium text-foreground">person who applied</strong>.
      Only the second kind can witness that a job is not being worked, so no quantity of the
      first will ever produce the stronger warning.
    </p>

    <div class="flex flex-col gap-6">
      {#each [{ heading: 'The shape of the posting', items: structural }, { heading: 'What happened to applicants', items: outcome }] as group (group.heading)}
        <div class="flex flex-col gap-3">
          <h3 class="text-sm font-semibold tracking-tight">{group.heading}</h3>
          <ul class="flex flex-col gap-3">
            {#each group.items as s (s.code)}
              <li class="rounded-lg border border-border p-4">
                <p class="text-sm font-medium">{s.label}</p>
                <p class="mt-1 font-mono text-xs text-muted-foreground">{s.fact}</p>
                <p class="mt-2 text-sm leading-relaxed text-muted-foreground">{s.why}</p>
              </li>
            {/each}
          </ul>
        </div>
      {/each}
    </div>
  </section>

  <!-- The UI, as markup. -->
  <section class="flex flex-col gap-6">
    <SectionLabel text="what you actually see" />

    <div class="flex flex-col gap-3">
      <p class="text-sm text-muted-foreground">
        On a card — a hedged chip and how many criteria fired out of how many were checked:
      </p>
      <div class="flex flex-wrap items-center gap-3 rounded-lg border border-border p-4">
        {#each levels as l (l.chip)}
          <span
            class="inline-flex items-center gap-1.5 rounded-md border px-2 py-0.5 text-xs font-medium {toneClass[
              l.tone
            ]}"
          >
            {l.chip}
            <span class="tabular-nums opacity-70">{l.scale}</span>
          </span>
        {/each}
      </div>
      <ul class="flex flex-col gap-1.5 text-sm text-muted-foreground">
        {#each levels as l (l.chip)}
          <li><strong class="font-medium text-foreground">{l.chip}</strong> — {l.when}.</li>
        {/each}
      </ul>
    </div>

    <div class="flex flex-col gap-3">
      <p class="text-sm text-muted-foreground">
        On the job page — the full checklist, <strong class="font-medium text-foreground"
          >including what we do not know</strong
        >. The empty rows are the point: they tell you why the warning is not stronger.
      </p>
      <section class="rounded-lg border border-border p-4">
        <div class="mb-3 flex items-baseline justify-between gap-4">
          <h3 class="text-sm font-semibold tracking-tight">
            Signs this posting may be inactive
          </h3>
          <span class="text-xs tabular-nums text-muted-foreground">2 / 4</span>
        </div>
        <ul class="flex flex-col gap-2">
          {#each demoChecklist as row (row.label)}
            <li class="flex items-start gap-2.5 text-sm">
              {#if row.fired}
                <Check class="mt-0.5 size-4 shrink-0 text-amber-600 dark:text-amber-400" />
              {:else}
                <Minus class="mt-0.5 size-4 shrink-0 text-muted-foreground/50" />
              {/if}
              <span class="flex min-w-0 flex-col">
                <span class={row.fired ? '' : 'text-muted-foreground'}>{row.label}</span>
                <span class="text-xs text-muted-foreground">{row.detail}</span>
              </span>
            </li>
          {/each}
        </ul>
        <p class="mt-3 text-xs text-muted-foreground">
          These are observations about the posting, not a claim about the employer.
        </p>
      </section>
    </div>
  </section>

  <!-- How a report becomes evidence. -->
  <section class="flex flex-col gap-5">
    <SectionLabel text="your part" />
    <p class="max-w-2xl text-sm leading-relaxed text-muted-foreground">
      The strongest signal is the one only you can give: you applied, and nobody ever answered.
      Reporting it takes one date.
    </p>
    <NumberedGrid
      items={[
        {
          n: '01',
          title: 'Report the silence',
          body: 'On the job, choose Report → “No response”. It asks when you applied — that date is what separates a real silence from impatience.',
        },
        {
          n: '02',
          title: 'It waits before it counts',
          body: 'Your report carries no weight until the same span has passed that a tracked application gets. Withdraw it if the employer eventually answers, and it stops counting.',
        },
        {
          n: '03',
          title: 'It stays anonymous',
          body: `Only a count leaves your report, and only once ${WITNESS_GATE} different people have contributed — a count of one would point straight at you.`,
        },
      ]}
    />
    <p class="max-w-2xl text-sm leading-relaxed text-muted-foreground">
      Connect a mailbox and it happens without you: freehire sees the reply arrive, or sees that
      it never did. Without a connected mailbox nothing is counted, because we could not tell
      silence from a gap in our own data.
    </p>
    <div class="flex flex-wrap gap-3">
      <Button href={resolve('/features/inbox')} variant="outline">How the inbox works</Button>
    </div>
  </section>

  <!-- Honest limits. -->
  <section class="flex flex-col gap-4">
    <SectionLabel text="where it is blind" />
    <ul class="flex max-w-3xl flex-col gap-3 text-sm leading-relaxed text-muted-foreground">
      <li>
        <strong class="font-medium text-foreground">We only check boards we crawl.</strong> If a
        company's own careers site is not one of them, its postings are not judged on that
        criterion at all — absence of proof is not proof.
      </li>
      <li>
        <strong class="font-medium text-foreground">Age counts from when we first saw it.</strong>
        Sources that refresh a posting's date do not fool it, but a job that existed before we
        started crawling looks younger than it is.
      </li>
      <li>
        <strong class="font-medium text-foreground"
          >Agencies and consultancies fire the board criterion honestly.</strong
        > They advertise roles that belong to their clients, so those roles are genuinely absent
        from their own board. This is why one criterion is never enough on its own.
      </li>
      <li>
        <strong class="font-medium text-foreground">A quiet employer is not a fake job.</strong>
        Recruiters go silent for ordinary reasons, which is why a silence needs a second person and
        a second signal before it says anything.
      </li>
    </ul>
  </section>

  <!-- FAQ; shares GHOST_FAQ with the page's JSON-LD. -->
  <section class="flex flex-col gap-5">
    <SectionLabel text="questions" />
    <dl class="flex flex-col gap-5">
      {#each GHOST_FAQ as item (item.question)}
        <div class="flex flex-col gap-1.5">
          <dt class="text-sm font-medium">{item.question}</dt>
          <dd class="max-w-3xl text-sm leading-relaxed text-muted-foreground">{item.answer}</dd>
        </div>
      {/each}
    </dl>
  </section>

  <section class="flex flex-col items-start gap-4 rounded-lg border border-border p-6">
    <h2 class="text-lg font-semibold tracking-tight">Stop applying into the void</h2>
    <p class="max-w-2xl text-sm leading-relaxed text-muted-foreground">
      The catalogue is open and needs no account. Tracking your applications is what makes the
      signal sharper — for you, and for whoever reads the posting after you.
    </p>
    <div class="flex flex-wrap gap-3">
      <Button href={resolve('/jobs')} variant="primary">Browse jobs</Button>
      <Button href={resolve('/my/tracking')} variant="ghost">Your tracking board</Button>
    </div>
  </section>
</article>
