<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import Seo from '$lib/components/Seo.svelte';
  import { Avatar, SectionLabel } from '$lib/ui';
  import { contributionSummary } from '$lib/contributors';
  import type { ContributorEntry } from '$lib/contributors';
  import { breadcrumbJsonLd, jsonLdScript } from '$lib/seo';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const origin = $derived(page.url.origin);
  const canonical = $derived(`${origin}/contributors`);

  // Counted, never written down: a hand-typed figure beside a generated list is a second
  // answer to the same question, and it is the one that goes stale.
  const people = $derived(data.maintainers.length + data.contributors.length);

  const description = $derived(
    `The ${people} people who build freehire — every merged pull request and every issue opened, straight from the repository.`,
  );

  const jsonLd = $derived(
    jsonLdScript([
      breadcrumbJsonLd([
        { name: 'freehire', url: `${origin}/` },
        { name: 'Contributors', url: canonical },
      ]),
    ]),
  );

  const since = (entry: ContributorEntry) =>
    new Date(entry.firstContributionAt).toLocaleDateString('en', {
      month: 'short',
      year: 'numeric',
    });
</script>

<Seo title="Contributors — the people who build freehire" {description} {canonical} />

<svelte:head>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD built by jsonLdScript, which escapes `<`; raw injection is the only way to emit a structured-data <script> -->
  {@html jsonLd}
</svelte:head>

{#snippet contributorCard(entry: ContributorEntry)}
  <a
    href={resolve('/contributors/[login]', { login: entry.login })}
    class="group flex items-start gap-4 rounded-xl border border-border bg-background p-5 transition-colors hover:border-foreground/25 hover:bg-muted/40"
  >
    <Avatar name={entry.login} src={entry.avatarUrl} size="lg" />
    <div class="min-w-0">
      <p class="truncate font-semibold tracking-tight group-hover:underline">{entry.login}</p>
      <p class="mt-1 text-sm text-muted-foreground">{contributionSummary(entry)}</p>
      <p class="mt-2 font-mono text-xs uppercase tracking-wide text-muted-foreground">
        since {since(entry)}
      </p>
    </div>
  </a>
{/snippet}

<div class="mx-auto w-full max-w-4xl px-4 py-10 sm:py-14">
  <header class="dot-grid -mx-4 mb-12 px-4 pb-8 pt-4">
    <SectionLabel text="contributors" />
    <h1 class="mt-4 text-4xl font-semibold tracking-tighter sm:text-5xl">
      {people} people build freehire.
    </h1>
    <p class="mt-4 max-w-2xl text-lg leading-relaxed text-muted-foreground">
      Everyone below has merged code or opened an issue against the repository. The list comes
      straight from GitHub — nobody is added by hand, and nobody is left out by forgetting.
    </p>
  </header>

  {#if data.maintainers.length > 0}
    <section class="mb-14">
      <SectionLabel text="maintained by" />
      <div class="mt-6 grid gap-3 sm:grid-cols-2">
        {#each data.maintainers as entry (entry.login)}
          {@render contributorCard(entry)}
        {/each}
      </div>
    </section>
  {/if}

  <section class="mb-14">
    <SectionLabel text="contributed by" />
    <h2 class="mt-3 text-xl font-semibold tracking-tight">Most recent first</h2>
    <p class="mb-6 mt-1 text-sm text-muted-foreground">
      Ordered by when someone last contributed, not by how much — so the newest arrival is at the
      top, where they should be.
    </p>

    <div class="grid gap-3 sm:grid-cols-2">
      {#each data.contributors as entry (entry.login)}
        {@render contributorCard(entry)}
      {/each}
    </div>
  </section>

  <section class="rounded-xl border border-border bg-muted/30 p-6 sm:p-8">
    <h2 class="text-xl font-semibold tracking-tight">Your name goes here next.</h2>
    <p class="mt-2 max-w-2xl text-muted-foreground">
      An opened issue counts as much as merged code. Pick something that annoys you about freehire
      and tell us, or fix it.
    </p>
    <div class="mt-5 flex flex-wrap gap-3">
      <a
        href={resolve('/contribute')}
        class="rounded-lg bg-foreground px-4 py-2 text-sm font-medium text-background transition-opacity hover:opacity-90"
      >
        How to contribute
      </a>
      <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- external repository, not a SvelteKit route -->
      <a
        href="https://github.com/strelov1/freehire/issues"
        class="rounded-lg border border-border px-4 py-2 text-sm font-medium transition-colors hover:bg-background"
        rel="noopener"
        target="_blank"
      >
        Open an issue ↗
      </a>
    </div>
  </section>
</div>
