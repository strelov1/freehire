<script lang="ts">
  import { GraduationCap } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { api } from '$lib/api';
  import { mentorFiltersToQuery } from '$lib/mentorship';
  import type { Mentor } from '$lib/types';

  // The entry point from a vacancy and from a company page: a route to that employer's
  // mentors, rendered only where there is one.
  let { companySlug, companyName }: { companySlug: string; companyName: string } = $props();

  // Asked from the DIRECTORY narrowed to this company, never from a separate "has a
  // mentor?" endpoint. A second way to ask is a second copy of the publication predicate —
  // approved, unpaused, not withdrawn — and two copies of a predicate drift.
  //
  // Asked in the BROWSER rather than in the route's `load`, which is the one thing that
  // makes this affordable: the job page is the busiest surface here and roughly three
  // quarters of this host's traffic is crawlers, so a server-side call would spend an API
  // request on every bot fetch to answer a question no bot acts on. The cost is that the
  // block appears after hydration; it sits below the header's own content, so what it
  // shifts is the description rather than anything a visitor is mid-click on.
  let mentors = $state<Mentor[]>([]);

  // The same filter this block asked with, so the directory it opens shows exactly the
  // people it counted — built once rather than spelled twice.
  const companyFilter = $derived(
    mentorFiltersToQuery({ company: companySlug, topic: '', language: '' }),
  );
  const directoryHref = $derived(`${resolve('/mentors')}?${companyFilter}`);

  // The same beta gate the routes hold, on the entry point: a block that leads somewhere
  // answering 404 is worse than no block. The routes are what actually close the door —
  // this only stops offering it, which is why the predicate is duplicated deliberately
  // rather than trusted here.
  const inBeta = $derived(Boolean(page.data.user?.beta_tester));

  $effect(() => {
    if (!companySlug || !inBeta) return;
    let live = true;
    // Failure is silence. This block is an extra route to something, not the page's
    // subject, and a vacancy must not show an error because an aside could not load.
    void api
      .listMentors(new URLSearchParams(companyFilter))
      .then((found) => {
        if (live) mentors = found;
      })
      .catch(() => {});
    return () => {
      live = false;
    };
  });
</script>

{#if inBeta && mentors.length > 0}
  <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve() applied to the /mentors path; the rule can't see through the appended query -->
  <a href={directoryHref} class="border-border hover:bg-muted/50 flex items-center gap-3 rounded-lg border p-3 text-sm transition-colors">
    <GraduationCap class="text-muted-foreground size-5 shrink-0" />
    <span>
      <span class="font-medium">
        {mentors.length === 1
          ? `Talk to someone at ${companyName}`
          : `Talk to one of ${mentors.length} people at ${companyName}`}
      </span>
      <span class="text-muted-foreground block">
        Book half an hour before you apply.
      </span>
    </span>
  </a>
{/if}
