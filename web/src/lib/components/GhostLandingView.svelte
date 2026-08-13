<script lang="ts">
  import { resolve } from '$app/paths';
  import { Button } from '$lib/ui';
  import Disclosure from '$lib/components/ghost/Disclosure.svelte';
  import GateMatrix from '$lib/components/ghost/GateMatrix.svelte';
  import GhostChecklist from '$lib/components/GhostChecklist.svelte';
  import Prevalence from '$lib/components/ghost/Prevalence.svelte';
  import SignalDiagram from '$lib/components/ghost/SignalDiagram.svelte';
  import { NumberedGrid, SectionLabel } from '$lib/ui';
  import { CRITERIA } from '$lib/ghost';
  import type { Ghost } from '$lib/generated/contracts';
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
  // Two structural criteria and nothing from applicants — the commonest thing a reader
  // arrives having just seen, and the state the matrix says cannot go further on its own.
  const example: Ghost = {
    level: 'possible',
    criteria: ['evergreen_posting', 'ats_absent'],
    criteria_total: CRITERIA.length,
    ats_checked_at: new Date(Date.now() - 2 * 86_400_000).toISOString(),
  };

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
  <header class="flex flex-col items-start gap-10 lg:flex-row lg:gap-16">
    <div class="flex max-w-2xl flex-col gap-6">
      <SectionLabel text="ghost jobs" />
      <h1 class="text-3xl font-semibold tracking-tight sm:text-4xl">
        Some postings are not being filled. freehire tells you which, and why it thinks so.
      </h1>
      <p class="text-base leading-relaxed text-muted-foreground">
        You can't tell by looking at the page. An hour spent on one is work you'll never
        hear back about. So freehire checks two things it can actually see: how the posting
        behaves, and what happened to people who applied.
      </p>

      <!-- Two lines, not a section. The limit on the claim is the feature's most
           important property and has to be above the mechanics, but it earns a couplet
           there — as a full section it was the third block of grey before the first CTA. -->
      <dl class="flex flex-col gap-1.5 border-l-2 border-border pl-4 text-sm leading-relaxed">
        <div>
          <dt class="inline font-medium">We check —</dt>
          <dd class="inline text-muted-foreground">
            how long the job has been up, how often it's reposted, whether the company's own
            careers page lists it, and whether people who applied got an answer.
          </dd>
        </div>
        <div>
          <dt class="inline font-medium text-muted-foreground">We never say —</dt>
          <dd class="inline text-muted-foreground">
            that a company isn't really hiring. That's about what someone meant to do, and
            we can't see that.
          </dd>
        </div>
      </dl>

      <div class="flex flex-wrap gap-3">
        <Button href={resolve('/jobs')} variant="primary" size="lg">Browse jobs</Button>
        <Button href={resolve('/my/tracking')} variant="ghost" size="lg">
          Track your applications
        </Button>
      </div>
    </div>

    <div class="shrink-0"><Prevalence /></div>
  </header>

  <!-- The four criteria, each with its own picture. -->
  <section class="flex flex-col gap-6">
    <SectionLabel text="what lights the mark" />
    <p class="max-w-2xl text-sm leading-relaxed text-muted-foreground">
      Two of them look at the <strong class="font-medium text-foreground">posting</strong>.
      Two look at what happened to
      <strong class="font-medium text-foreground">people who applied</strong>. Only the
      second kind can tell you nobody is working on the job.
    </p>

    <div class="grid gap-x-6 gap-y-8 sm:grid-cols-2">
      {#each groups as group (group.heading)}
        <div class="flex flex-col gap-4">
          <h3 class="text-sm font-semibold tracking-tight">{group.heading}</h3>
          {#each group.items as s (s.code)}
            <!-- Diagram, name, one sentence, and the rest behind a control. The example
                 `fact` line used to sit here too and was the worst thing on the page: for
                 `ats_absent` it repeated the criterion's name verbatim, directly under the
                 criterion's name, directly under a diagram already showing it. It lives in
                 the disclosure now, where it belongs with the full account. -->
            <div class="flex flex-1 flex-col gap-3 rounded-lg border border-border p-4">
              <SignalDiagram code={s.code} />
              <div class="flex flex-col gap-1.5">
                <p class="text-sm font-medium">{s.label}</p>
                <p class="text-sm leading-relaxed text-muted-foreground">{s.gist}</p>
                <Disclosure summary="Why this counts" srSuffix={s.label} class="mt-1">
                  <div
                    class="mt-2 flex flex-col gap-2 border-l border-border pl-3 text-sm leading-relaxed text-muted-foreground"
                  >
                    <p class="font-mono text-xs">{s.fact}</p>
                    <p>{s.why}</p>
                  </div>
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
  <!-- The matrix alone. A toggle-driven preview of the badge stood here and was cut: it
       was the heaviest thing on the page, and the table already says what it demonstrated
       — the strongest wording sits in one corner, and the posting alone cannot reach it. -->
  <section class="flex flex-col gap-6">
    <SectionLabel text="why the level is what it is" />

    <GateMatrix />

    <!-- The real components, fed an illustrative payload — never a screenshot and never a
         copy of their markup, which goes stale the first time they are redesigned. This is
         what the toggle-driven version was built on; only the toggles are gone. -->
    <div class="flex max-w-xl flex-col gap-2 rounded-lg border border-border p-4">
      <span class="text-xs text-muted-foreground">This is what a marked job shows you</span>
      <GhostChecklist ghost={example} />
    </div>

    <p class="text-xs text-muted-foreground">
      These are observations about the posting, not a claim about the employer.
    </p>
  </section>

  <!-- How a person's experience becomes evidence. The waiting period and the anonymity
       gate are not restated here — the reports diagram above draws both, and saying it
       twice says it the second time more weakly. -->
  <section class="flex flex-col gap-5">
    <SectionLabel text="your part" />
    <p class="max-w-2xl text-sm leading-relaxed text-muted-foreground">
      The strongest signal is one only you can give: you applied and nobody answered. There
      are two ways it reaches us.
    </p>
    <NumberedGrid
      items={[
        {
          n: '01',
          title: 'Report it yourself',
          body: 'On the job, pick Report → “No response”. It asks one thing: when you applied. That date is what tells a real silence apart from impatience. You can take the report back if the company answers later.',
        },
        {
          n: '02',
          title: 'Or connect a mailbox',
          body: 'Then it happens on its own: freehire sees the reply arrive, or sees that it never did. Without a connected mailbox nothing is counted — we could not tell silence from a gap in our own data.',
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
        <strong class="font-medium text-foreground">We only check pages we crawl.</strong>
        If we don't crawl a company's careers page, its jobs aren't judged on that criterion
        at all. Not finding something isn't proof.
      </li>
      <li>
        <strong class="font-medium text-foreground">Age counts from when we first saw it.</strong>
        Sources that refresh the date don't fool us, but a job that was up before we started
        crawling looks younger than it is.
      </li>
      <li>
        <strong class="font-medium text-foreground">Agencies trip this honestly.</strong>
        They advertise roles that belong to their clients, so those roles really are missing from
        their own page. That's why one criterion is never enough.
      </li>
      <li>
        <strong class="font-medium text-foreground">A quiet company isn't a fake job.</strong>
        Recruiters go quiet for ordinary reasons. That's why a silence needs a second person and
        a second signal before it says anything.
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
