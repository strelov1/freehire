<script lang="ts">
  import { resolve } from '$app/paths';
  import { Button } from '$lib/ui';
  import Disclosure from '$lib/components/ghost/Disclosure.svelte';
  import GateMatrix from '$lib/components/ghost/GateMatrix.svelte';
  import GhostSandbox from '$lib/components/ghost/GhostSandbox.svelte';
  import Prevalence from '$lib/components/ghost/Prevalence.svelte';
  import SignalDiagram from '$lib/components/ghost/SignalDiagram.svelte';
  import NumberedGrid from '$lib/components/NumberedGrid.svelte';
  import SectionLabel from '$lib/components/SectionLabel.svelte';
  import { GHOST_FAQ } from '$lib/ghostFaq';
  import { GHOST_SIGNALS } from '$lib/ghostSignals';

  // The page a reader reaches from the "How this works" link on a marked job. They arrive
  // mid-question, having just seen a badge they did not understand, and the two things
  // they want are which criteria fired and why the warning is not stronger. Both are now
  // drawn rather than described: the criteria carry a diagram each, and the level rule is
  // a matrix plus a preview the reader can drive.
  //
  // The signals still render from the same array the product's checklist reads, so this
  // page cannot fall behind the vocabulary — ghostSignals.test.ts fails if a criterion
  // joins without an explanation, and the diagram registry fails to COMPILE if it joins
  // without an illustration. See hire-features-landing-space.
  const groups = $derived([
    {
      heading: 'The shape of the posting',
      items: GHOST_SIGNALS.filter((s) => s.tier === 'structural'),
    },
    {
      heading: 'What happened to applicants',
      items: GHOST_SIGNALS.filter((s) => s.tier === 'outcome'),
    },
  ]);
</script>

<article class="flex flex-col gap-16">
  <!-- Hero. The limit on the claim sits here rather than in a section of its own: it is
       the feature's most important property, not a disclaimer, and a reader who stops
       after the first screen must still have read it. -->
  <header class="flex flex-col gap-6">
    <SectionLabel text="ghost jobs" />
    <h1 class="max-w-3xl text-3xl font-semibold tracking-tight sm:text-4xl">
      Some postings are not being filled. freehire tells you which, and why it thinks so.
    </h1>
    <p class="max-w-2xl text-base leading-relaxed text-muted-foreground">
      You cannot tell from the page, and every hour spent on one is unpaid work you will
      never hear about. So freehire watches two things it can actually check: how a posting
      behaves over time, and what happened to people who applied.
    </p>

    <dl
      class="flex max-w-3xl flex-col gap-2 border-l-2 border-border pl-4 text-sm leading-relaxed"
    >
      <div>
        <dt class="inline font-medium">What we can check —</dt>
        <dd class="inline text-muted-foreground">
          “open 240 days, reposted 13 times, seven copies live at once, and absent from the
          employer's own careers board.” Every part is verifiable, and every part is shown
          to you.
        </dd>
      </div>
      <div>
        <dt class="inline font-medium text-muted-foreground">What we never claim —</dt>
        <dd class="inline text-muted-foreground">
          “this company is not really hiring.” That is a claim about somebody's intent,
          which no amount of data here can observe.
        </dd>
      </div>
    </dl>

    <Prevalence />

    <div class="flex flex-wrap gap-3">
      <Button href={resolve('/jobs')} variant="primary" size="lg">Browse jobs</Button>
      <Button href={resolve('/my/tracking')} variant="ghost" size="lg">
        Track your applications
      </Button>
    </div>
  </header>

  <!-- The four criteria, each with its own picture. -->
  <section class="flex flex-col gap-6">
    <SectionLabel text="what lights the mark" />
    <p class="max-w-2xl text-sm leading-relaxed text-muted-foreground">
      Two describe the <strong class="font-medium text-foreground">posting</strong>. Two
      describe what happened to a
      <strong class="font-medium text-foreground">person who applied</strong>. Only the
      second kind can witness that a job is not being worked.
    </p>

    <div class="grid gap-x-6 gap-y-8 sm:grid-cols-2">
      {#each groups as group (group.heading)}
        <div class="flex flex-col gap-4">
          <h3 class="text-sm font-semibold tracking-tight">{group.heading}</h3>
          {#each group.items as s (s.code)}
            <div class="flex flex-col gap-3 rounded-lg border border-border p-4">
              <SignalDiagram code={s.code} />
              <div class="flex flex-col gap-1.5">
                <p class="text-sm font-medium">{s.label}</p>
                <p class="font-mono text-xs text-muted-foreground">{s.fact}</p>
                <p class="text-sm leading-relaxed text-muted-foreground">{s.gist}</p>
                <Disclosure summary="The full account" class="mt-1">
                  <p
                    class="mt-2 border-l border-border pl-3 text-sm leading-relaxed text-muted-foreground"
                  >
                    {s.why}
                  </p>
                </Disclosure>
              </div>
            </div>
          {/each}
        </div>
      {/each}
    </div>
  </section>

  <!-- How the level is decided. The matrix states the rule; the sandbox lets the reader
       try to break it and find they cannot. -->
  <section class="flex flex-col gap-6">
    <SectionLabel text="why the level is what it is" />
    <p class="max-w-2xl text-sm leading-relaxed text-muted-foreground">
      Two separate gates, and the wording depends on how many pass. Nothing shows at all
      until at least two criteria fire — one on its own is ordinary.
    </p>

    <GateMatrix />

    <div class="mt-2 flex flex-col gap-3">
      <h3 class="text-sm font-semibold tracking-tight">Try it</h3>
      <p class="max-w-2xl text-sm leading-relaxed text-muted-foreground">
        Below is the interface the job page renders, driven by what you switch on. See how
        far posting shape alone can take it.
      </p>
      <GhostSandbox />
      <p class="text-xs text-muted-foreground">
        These are observations about the posting, not a claim about the employer.
      </p>
    </div>
  </section>

  <!-- How a person's experience becomes evidence. The waiting period and the anonymity
       gate are not restated here — the reports diagram above draws both, and saying it
       twice says it the second time more weakly. -->
  <section class="flex flex-col gap-5">
    <SectionLabel text="your part" />
    <p class="max-w-2xl text-sm leading-relaxed text-muted-foreground">
      The strongest signal is the one only you can give: you applied, and nobody ever
      answered. There are two ways it reaches us.
    </p>
    <NumberedGrid
      items={[
        {
          n: '01',
          title: 'Report it yourself',
          body: 'On the job, choose Report → “No response”. It asks one thing: when you applied. That date is what separates a real silence from impatience, and you can withdraw the report if the employer eventually answers.',
        },
        {
          n: '02',
          title: 'Or connect a mailbox',
          body: 'Then it happens without you: freehire sees the reply arrive, or sees that it never did. Without a connected mailbox nothing is counted, because we could not tell silence from a gap in our own data.',
        },
      ]}
    />
    <div class="flex flex-wrap gap-3">
      <Button href={resolve('/features/inbox')} variant="outline">How the inbox works</Button>
    </div>
  </section>

  <!-- Honest limits of coverage — a different thing from the limit on the claim, which is
       in the hero where a reader cannot miss it. -->
  <section class="flex flex-col gap-4">
    <SectionLabel text="where it is blind" />
    <ul class="flex max-w-3xl flex-col gap-3 text-sm leading-relaxed text-muted-foreground">
      <li>
        <strong class="font-medium text-foreground">We only check boards we crawl.</strong>
        If a company's own careers site is not one of them, its postings are not judged on that
        criterion at all — absence of proof is not proof.
      </li>
      <li>
        <strong class="font-medium text-foreground">Age counts from when we first saw it.</strong>
        Sources that refresh a posting's date do not fool it, but a job that existed before we
        started crawling looks younger than it is.
      </li>
      <li>
        <strong class="font-medium text-foreground"
          >Agencies fire the board criterion honestly.</strong
        > They advertise roles that belong to their clients, so those roles are genuinely absent
        from their own board. This is why one criterion is never enough on its own.
      </li>
      <li>
        <strong class="font-medium text-foreground">A quiet employer is not a fake job.</strong>
        Recruiters go silent for ordinary reasons, which is why a silence needs a second person
        and a second signal before it says anything.
      </li>
    </ul>
  </section>

  <!-- FAQ; shares GHOST_FAQ with the page's JSON-LD. Collapsed with <details>, so every
       answer is still in the served HTML and the structured data cannot disagree with it. -->
  <section class="flex flex-col gap-4">
    <SectionLabel text="questions" />
    <div class="flex flex-col divide-y divide-border border-y border-border">
      {#each GHOST_FAQ as item (item.question)}
        <Disclosure summary={item.question} class="py-3">
          <p class="mt-2 max-w-3xl text-sm leading-relaxed text-muted-foreground">
            {item.answer}
          </p>
        </Disclosure>
      {/each}
    </div>
  </section>

  <section class="flex flex-col items-start gap-4 rounded-lg border border-border p-6">
    <h2 class="text-lg font-semibold tracking-tight">Stop applying into the void</h2>
    <p class="max-w-2xl text-sm leading-relaxed text-muted-foreground">
      The catalogue is open and needs no account. Tracking your applications is what makes
      the signal sharper — for you, and for whoever reads the posting after you.
    </p>
    <div class="flex flex-wrap gap-3">
      <Button href={resolve('/jobs')} variant="primary">Browse jobs</Button>
      <Button href={resolve('/my/tracking')} variant="ghost">Your tracking board</Button>
    </div>
  </section>
</article>
