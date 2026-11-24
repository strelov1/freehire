<script lang="ts">
  import { PUBLIC_MATCHES } from '$app/env/public';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { ArrowRight, Bookmark, Building2, ExternalLink, EyeOff } from '@lucide/svelte';
  import Seo from '#lib/components/Seo.svelte';
  import { matchesEnabled } from '#lib/features.js';
  import { Button } from '#lib/ui/index.js';

  const matchesOn = matchesEnabled({ PUBLIC_MATCHES });
  const canonical = $derived(`${page.url.origin}${resolve('/features/matches')}`);
  const actions = [
    { icon: Bookmark, title: 'Save', detail: 'Keep a vacancy so you can come back to it later.' },
    { icon: EyeOff, title: 'Hide or skip', detail: 'Remove this vacancy from your feed.' },
    { icon: ExternalLink, title: 'Apply', detail: 'Open the application on freehire or the site hosting it.' },
    { icon: Building2, title: 'Hide company', detail: 'Remove all vacancies from that employer from your feed.' },
  ];
</script>

<Seo
  title="Job Matches — a personal job feed | freehire"
  description="Use your job preferences or a saved search to find relevant openings, see why each role might fit, and decide what to save, skip, or apply for."
  {canonical}
/>

<div class="mx-auto w-full max-w-6xl px-4 py-10 sm:px-6 sm:py-16">
  <section class="grid items-center gap-8 lg:grid-cols-2 lg:gap-12" aria-labelledby="matches-title">
    <div>
      <h1 id="matches-title" class="text-4xl font-semibold tracking-tight sm:text-5xl">Job Matches</h1>
      <p class="mt-4 max-w-xl text-lg text-muted-foreground">
        A personal job feed based on your profile or a saved search. See why a role might fit and what to check before
        you apply.
      </p>
      <p class="mt-4 text-sm font-medium">
        {matchesOn ? 'Included in Pro and Ultra.' : 'Matches is currently unavailable.'}
      </p>
      <div class="mt-6 flex flex-wrap gap-3">
        {#if matchesOn}
          <Button href={resolve('/matches')} variant="cta">
            Open Matches <ArrowRight class="size-4" aria-hidden="true" />
          </Button>
          <Button href={resolve('/pricing')} variant="outline">Compare plans</Button>
        {:else}
          <Button href={resolve('/jobs')} variant="cta">Browse jobs</Button>
        {/if}
      </div>
    </div>

    <figure class="rounded-2xl border border-border bg-card p-5 sm:p-6">
      <figcaption class="text-sm text-muted-foreground">Example match</figcaption>
      <p class="mt-2 text-xl font-semibold">Backend Engineer</p>
      <dl class="mt-4 grid gap-3 border-t border-border pt-4 text-sm">
        <div>
          <dt class="font-medium">Why it fits</dt>
          <dd class="mt-1 text-muted-foreground">The role uses Go and PostgreSQL, two skills in your profile.</dd>
        </div>
        <div>
          <dt class="font-medium">What may not fit</dt>
          <dd class="mt-1 text-muted-foreground">The team expects two days a week in the office.</dd>
        </div>
        <div>
          <dt class="font-medium">What’s unknown</dt>
          <dd class="mt-1 text-muted-foreground">The posting doesn’t list a salary range.</dd>
        </div>
      </dl>
    </figure>
  </section>

  <section class="mt-12 border-t border-border pt-8 sm:mt-16" aria-labelledby="how-matches-work">
    <h2 id="how-matches-work" class="text-2xl font-semibold tracking-tight">How it works</h2>
    <ol class="mt-5 grid list-decimal gap-5 pl-5 marker:font-medium sm:grid-cols-3 sm:gap-8">
      <li>
        <h3 class="font-semibold">Choose a source</h3>
        <p class="mt-1 text-sm text-muted-foreground">
          Use the job preferences in your profile or pick a saved search. Matches respects the search filters.
        </p>
      </li>
      <li>
        <h3 class="font-semibold">See what’s new first</h3>
        <p class="mt-1 text-sm text-muted-foreground">
          Jobs added since you last reviewed new matches come first. Your profile helps rank the results. Without enough
          skills to rank by match, the feed sorts by date.
        </p>
      </li>
      <li>
        <h3 class="font-semibold">Review at your pace</h3>
        <p class="mt-1 text-sm text-muted-foreground">
          Scan the list or switch to Swipe to review one vacancy at a time. You can undo a skip or save in Swipe.
        </p>
      </li>
    </ol>
  </section>

  <section class="mt-12 border-t border-border pt-8 sm:mt-16" aria-labelledby="matches-actions">
    <h2 id="matches-actions" class="text-2xl font-semibold tracking-tight">What you can do with a match</h2>
    <dl class="mt-5 grid gap-3 sm:grid-cols-2">
      {#each actions as { icon: Icon, title, detail } (title)}
        <div class="rounded-xl border border-border bg-card p-4">
          <dt class="flex items-center gap-2 font-medium">
            <Icon class="size-4 text-muted-foreground" aria-hidden="true" />
            {title}
          </dt>
          <dd class="mt-1 text-sm text-muted-foreground">{detail}</dd>
        </div>
      {/each}
    </dl>
  </section>
</div>
