<script lang="ts">
  import { formatSalary, summaryFacets } from '#lib/enrichment.js';
  import { applyQuestionsLabel, equityLabel } from '#lib/jobFacts.js';
  import type { Job, MatchAnalysisResponse } from '#lib/types.js';
  import { Chip } from '#lib/ui/index.js';
  import CountryFlagStack from './CountryFlagStack.svelte';
  import JobMatch from './JobMatch.svelte';
  import JobSourceRow from './JobSourceRow.svelte';
  import VoteControl from './VoteControl.svelte';

  // The job page's left rail: the match score, the pay and what applying costs, the
  // posting's facets, and its source and votes. `closed` withholds the question count.
  let { job, matchAnalysis, closed }: { job: Job; matchAnalysis: MatchAnalysisResponse | null; closed: boolean } =
    $props();

  const e = $derived(job.enrichment ?? {});
  const salary = $derived(formatSalary(e, 'standard'));
  const equity = $derived(equityLabel(job.equity));
  // A closed posting takes no applications, so what its form asks is beside the point.
  const questions = $derived(closed ? null : applyQuestionsLabel(job.apply_questions));
  const facets = $derived(summaryFacets(job));
</script>

<aside class="w-full shrink-0 lg:col-start-1 lg:row-span-3 lg:row-start-1">
  <!-- `top-20`, not `top-6`: the site header is `sticky top-0 h-14` and opaque, so a
       card pinned 24px from the viewport spends the whole read with its first 32px —
       the border, the padding, and the top of the match score — behind it. 80px is
       the header's 56 plus the same 24 of air the card was asking for. -->
  <div class="sticky top-20 flex flex-col gap-4 rounded-xl border border-border bg-card p-4">
    <JobMatch {job} {matchAnalysis} />

    <!-- The pay and what applying costs, together: equity is the other half of the
         pay (a posting may offer it with no salary figure), and the question count is
         what stands between the reader and the Apply button. Each renders only when
         known — absence is "not stated", never "none". -->
    {#if salary || equity || questions}
      <div class="flex flex-col gap-2 border-t border-border pt-4 first:border-t-0 first:pt-0">
        {#if salary}
          <p class="text-xl font-semibold tabular-nums tracking-tight">{salary}</p>
        {/if}
        {#if equity || questions}
          <div class="flex flex-wrap gap-1.5">
            {#if equity}<Chip>{equity}</Chip>{/if}
            {#if questions}<Chip>{questions}</Chip>{/if}
          </div>
        {/if}
      </div>
    {/if}

    {#if facets.length}
      <dl class="flex flex-col gap-2 border-t border-border pt-4 text-sm first:border-t-0 first:pt-0">
        {#each facets as facet (facet.label)}
          <div class="flex items-baseline justify-between gap-3">
            <dt class="shrink-0 text-muted-foreground">{facet.label}</dt>
            {#if facet.values.every((v) => v.flag)}
              <!-- Flag facet (Country): an overlapping cluster of round flags,
                   right-aligned, each linking to its filter. The stack laps the
                   flags over one another so a many-country remote role stays a
                   single compact row instead of wrapping. -->
              <dd class="flex min-w-0 justify-end text-base">
                <!-- `labelClass` matches the sibling `dd` below (text-sm font-medium,
                     ordinary foreground) — the component's own default is a muted
                     caption sized for the browse card and company panel instead. -->
                <CountryFlagStack codes={facet.values.map((v) => v.flag ?? '')} link labelClass="text-sm font-medium" />
              </dd>
            {:else}
              <dd class="min-w-0 break-words text-right font-medium">
                {#each facet.values as v, i (v.text)}{#if i > 0},
                  {/if}{#if v.href}<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- internal filter link from filterHref; query-only, no route to resolve --><a
                      href={v.href}
                      class="hover:text-foreground hover:underline">{v.text}</a
                    >{:else}{v.text}{/if}{/each}
              </dd>
            {/if}
          </div>
        {/each}
      </dl>
    {/if}

    <!-- One quiet footer line: where the posting came from (with the control to stop seeing
         that source) and the votes. They were two divided bands, each holding one control. -->
    <div
      class="flex flex-wrap items-center justify-between gap-3 border-t border-border pt-4 first:border-t-0 first:pt-0"
    >
      <JobSourceRow source={job.source} jobUrl={job.url} manuallyAdded={job.manually_added} />
      <div class="flex shrink-0">
        <!-- Keyed on the slug so a client-side navigation to another job remounts
             the control and re-seeds its counts/highlight (JobView itself is not
             remounted on param change). -->
        {#key job.public_slug}
          <VoteControl
            target="job"
            slug={job.public_slug}
            upvoteCount={job.upvote_count ?? 0}
            downvoteCount={job.downvote_count ?? 0}
            myVote={job.my_vote ?? 0}
          />
        {/key}
      </div>
    </div>
  </div>
</aside>
