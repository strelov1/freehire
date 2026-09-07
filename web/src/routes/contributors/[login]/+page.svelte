<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import Seo from '$lib/components/Seo.svelte';
  import { Avatar, SectionLabel } from '$lib/ui';
  import { contributionSummary } from '$lib/contributors';
  import { breadcrumbJsonLd, jsonLdScript } from '$lib/seo';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const who = $derived(data.contributor);
  const origin = $derived(page.url.origin);
  const canonical = $derived(`${origin}/contributors/${who.login}`);

  const description = $derived(
    `${who.login} contributes to freehire, the open-source job search engine — ${contributionSummary(who)} since ${monthYear(who.firstContributionAt)}.`,
  );

  // The share text is what someone posts about their own work, so it reads as their
  // sentence and not as ours.
  const shareText = $derived(`I contribute to freehire, an open-source job search engine.`);
  const shareX = $derived(
    `https://x.com/intent/tweet?text=${encodeURIComponent(shareText)}&url=${encodeURIComponent(canonical)}`,
  );
  const shareLinkedIn = $derived(
    `https://www.linkedin.com/sharing/share-offsite/?url=${encodeURIComponent(canonical)}`,
  );

  const jsonLd = $derived(
    jsonLdScript([
      breadcrumbJsonLd([
        { name: 'freehire', url: `${origin}/` },
        { name: 'Contributors', url: `${origin}/contributors` },
        { name: who.login, url: canonical },
      ]),
    ]),
  );

  function monthYear(iso: string) {
    return new Date(iso).toLocaleDateString('en', { month: 'long', year: 'numeric' });
  }

  function day(iso: string) {
    return new Date(iso).toLocaleDateString('en', {
      day: 'numeric',
      month: 'short',
      year: 'numeric',
    });
  }

  const stats = $derived(
    [
      { label: 'merged pull requests', value: who.mergedPullRequests },
      { label: 'issues opened', value: who.openedIssues },
    ].filter((s) => s.value > 0),
  );
</script>

<Seo
  title="{who.login} — freehire contributors"
  {description}
  {canonical}
  image={`${canonical}/og.png`}
/>

<svelte:head>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD built by jsonLdScript, which escapes `<`; raw injection is the only way to emit a structured-data <script> -->
  {@html jsonLd}
</svelte:head>

<div class="mx-auto w-full max-w-3xl px-4 py-10 sm:py-14">
  <a
    href={resolve('/contributors')}
    class="font-mono text-xs uppercase tracking-widest text-muted-foreground hover:underline"
  >
    ← all contributors
  </a>

  <header class="mt-6 flex flex-wrap items-center gap-5">
    <Avatar name={who.login} src={who.avatarUrl} size="lg" class="size-20" />
    <div class="min-w-0">
      <h1 class="text-3xl font-semibold tracking-tighter sm:text-4xl">{who.login}</h1>
      <p class="mt-1 text-muted-foreground">
        {who.role === 'maintainer' ? 'Maintains' : 'Contributes to'} freehire since
        {monthYear(who.firstContributionAt)}
      </p>
      <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- external profile, not a SvelteKit route -->
      <a
        href="https://github.com/{who.login}"
        class="mt-1 inline-block text-sm text-muted-foreground hover:underline"
        rel="noopener"
        target="_blank"
      >
        github.com/{who.login} ↗
      </a>
    </div>
  </header>

  <dl class="mt-8 grid grid-cols-2 gap-px overflow-hidden rounded-xl border border-border bg-border">
    {#each stats as stat (stat.label)}
      <div class="bg-background p-5">
        <dt class="font-mono text-xs uppercase tracking-wide text-muted-foreground">
          {stat.label}
        </dt>
        <dd class="mt-2 text-3xl font-semibold tracking-tight tabular-nums">{stat.value}</dd>
      </div>
    {/each}
  </dl>

  {#if who.recentPullRequests.length > 0}
    <section class="mt-12">
      <SectionLabel text="merged" />
      <h2 class="mt-3 text-xl font-semibold tracking-tight">
        {who.recentPullRequests.length === who.mergedPullRequests
          ? 'Every pull request'
          : `The ${who.recentPullRequests.length} most recent pull requests`}
      </h2>

      <ul class="mt-6 divide-y divide-border rounded-xl border border-border">
        {#each who.recentPullRequests as pr (pr.number)}
          <li class="flex items-baseline justify-between gap-4 p-4">
            <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- links to GitHub, not a SvelteKit route -->
            <a href={pr.url} class="min-w-0 hover:underline" rel="noopener" target="_blank">
              <span class="font-mono text-xs text-muted-foreground">#{pr.number}</span>
              <span class="ml-2">{pr.title}</span>
            </a>
            <time
              class="shrink-0 font-mono text-xs uppercase tracking-wide text-muted-foreground"
              datetime={pr.mergedAt}
            >
              {day(pr.mergedAt)}
            </time>
          </li>
        {/each}
      </ul>
    </section>
  {/if}

  <section class="mt-12 rounded-xl border border-border bg-muted/30 p-6">
    <h2 class="text-lg font-semibold tracking-tight">Share this page</h2>
    <p class="mt-1 text-sm text-muted-foreground">
      It is yours — your work, your link, your face on the preview card.
    </p>
    <!-- eslint-disable svelte/no-navigation-without-resolve -- both are external share intents, not SvelteKit routes -->
    <div class="mt-4 flex flex-wrap gap-3">
      <a
        href={shareX}
        class="rounded-lg border border-border bg-background px-4 py-2 text-sm font-medium transition-colors hover:bg-muted"
        rel="noopener"
        target="_blank"
      >
        Share on X
      </a>
      <a
        href={shareLinkedIn}
        class="rounded-lg border border-border bg-background px-4 py-2 text-sm font-medium transition-colors hover:bg-muted"
        rel="noopener"
        target="_blank"
      >
        Share on LinkedIn
      </a>
    </div>
    <!-- eslint-enable svelte/no-navigation-without-resolve -->
  </section>
</div>
