<script lang="ts">
  import { resolve } from '$app/paths';
  import { api, ApiError } from '$lib/api';
  import { AsyncData } from '$lib/asyncData.svelte';
  import { isAuthenticated } from '$lib/auth.svelte';
  import type { Contribution, DiscordStatus, ResolvedLink } from '$lib/types';
  import { Badge, Button, Input } from '$lib/ui';
  import { timeAgo } from '$lib/utils';
  import States from './States.svelte';

  let url = $state('');
  let submitting = $state(false);
  let formError = $state<string | null>(null);
  // What became of the last link handed in — the intake answers with one of four outcomes
  // rather than an accepted/rejected pair, so there is a single piece of state to render from.
  let resolved = $state.raw<ResolvedLink | null>(null);

  const canSubmit = $derived(url.trim() !== '' && !submitting);

  // Load the caller's own contributions once the session is confirmed.
  const contribData = new AsyncData<Contribution[]>([]);
  $effect(() => {
    if (isAuthenticated()) void contribData.run(() => api.listMyContributions());
  });
  const status = $derived(contribData.status);
  const contributions = $derived(contribData.value);

  // Discord: same reward, a second surface — running `/contribute` from the freehire
  // Discord server instead of pasting a link here. Linking/unlinking the account lives
  // on Integrations now, alongside every other third-party connection; this is a
  // status line only.
  const discordData = new AsyncData<DiscordStatus | null>(null);
  $effect(() => {
    if (isAuthenticated()) void discordData.run(() => api.discordStatus());
  });
  const discord = $derived(discordData.value);

  // Where a row came from, appended to its line. Rows recorded before surfaces were tracked
  // (or by a client that sends no tag) read "unknown" and are better left unlabelled.
  function surfaceLabel(surface: string): string {
    return !surface || surface === 'unknown' || surface === 'web' ? '' : ` · via ${surface}`;
  }

  // A review row carries no board — show the link's host as its label instead.
  function hostOf(u: string): string {
    try {
      return new URL(u).host;
    } catch {
      return u;
    }
  }

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    if (!canSubmit) return;
    submitting = true;
    formError = null;
    resolved = null;
    try {
      resolved = await api.resolveJobLink(url.trim());
      url = '';
      // Refresh the list so a newly recorded board shows up without a manual reload.
      await contribData.run(() => api.listMyContributions());
    } catch (err) {
      // Only a malformed link is an error now: a board we already crawl, or one someone already
      // contributed, comes back as an ordinary outcome rather than a 409.
      formError =
        err instanceof ApiError ? err.message : 'Could not submit the link. Please try again.';
    } finally {
      submitting = false;
    }
  }
</script>

{#if !isAuthenticated()}
  <p class="py-12 text-center text-sm text-muted-foreground">Sign in to contribute a board.</p>
{:else}
  <div class="flex flex-col gap-6">
    <div class="flex flex-col gap-1">
      <h1 class="text-2xl font-semibold tracking-tight">Contribute a board</h1>
      <p class="text-sm text-muted-foreground">
        Found a company we don't cover yet? Paste any link from its ATS careers page — a vacancy
        or the board itself. If it's a board we don't crawl, we add it and pull in all of its
        jobs.
      </p>
    </div>

    {#if discord?.enabled}
      <div class="flex items-center justify-between gap-3 rounded-lg border border-border p-4 text-sm">
        <div class="flex flex-col gap-0.5">
          <span class="flex items-center gap-2 font-medium">
            Discord
            {#if discord.linked}
              <Badge variant="outline" class="border-brand-ring/40 text-brand-strong">Linked</Badge>
            {/if}
          </span>
          <span class="text-xs text-muted-foreground">
            {discord.linked
              ? 'Run /contribute in the freehire Discord server for the same reward.'
              : 'Link your account to run /contribute in the freehire Discord server for the same reward.'}
          </span>
        </div>
        <Button variant="secondary" size="sm" href={resolve('/my/integrations')}>
          {discord.linked ? 'Manage in Integrations' : 'Connect in Integrations'}
        </Button>
      </div>
    {/if}

    {#if resolved}
      <div class="rounded-lg border border-border bg-secondary/40 p-4 text-sm" role="status">
        {#if resolved.status === 'found'}
          We already have this one.
        {:else if resolved.status === 'tracked'}
          Added — and we already track this company, so the rest of its roles will follow on the
          next crawl.
        {:else if resolved.status === 'imported' && resolved.company_slug}
          Added — we already carry this company, and now we'll crawl this board of theirs too.
          <span class="font-medium">+1 AI credit.</span>
        {:else if resolved.status === 'imported'}
          Added, and this company is new to us — we'll start crawling its board.
          <span class="font-medium">+1 AI credit.</span>
        {:else if resolved.status === 'review'}
          Added. Its careers site isn't one we can crawl yet, so we'll check by hand whether we
          can pull the rest of its jobs. <span class="font-medium">Not credited yet.</span>
        {:else}
          We couldn't read that page. We'll check by hand whether we can pull its jobs — if we
          can, we'll award you a credit. <span class="font-medium">Not credited yet.</span>
        {/if}
        {#if resolved.public_slug}
          <a
            href={resolve('/jobs/[slug]', { slug: resolved.public_slug })}
            class="font-medium underline underline-offset-4"
          >
            View the job →
          </a>
        {/if}
        {#if resolved.company_slug}
          <a
            href={resolve('/companies/[slug]', { slug: resolved.company_slug })}
            class="font-medium underline underline-offset-4"
          >
            View the company →
          </a>
        {/if}
      </div>
    {/if}

    <form onsubmit={submit} class="flex flex-col gap-3 rounded-lg border border-border p-4">
      <label class="flex flex-col gap-1">
        <span class="text-sm font-medium">Job URL</span>
        <Input bind:value={url} type="url" placeholder="https://job-boards.greenhouse.io/…" class="w-full" />
      </label>
      {#if formError}
        <p class="text-sm text-destructive">{formError}</p>
      {/if}
      <div>
        <Button variant="primary" type="submit" disabled={!canSubmit}>
          {submitting ? 'Checking…' : 'Contribute'}
        </Button>
      </div>
    </form>

    <div class="flex flex-col gap-3">
      <h2 class="text-sm font-medium text-muted-foreground">My contributions</h2>
      {#if status === 'loading'}
        <States state="loading" />
      {:else if status === 'error'}
        <States state="error" message="Couldn't load your contributions." />
      {:else if contributions.length === 0}
        <States state="empty" message="No boards yet. Paste an ATS link to get started." />
      {:else}
        <ul class="flex flex-col divide-y divide-border rounded-lg border border-border">
          {#each contributions as c (c.id)}
            <li class="flex items-start justify-between gap-3 px-4 py-3">
              <div class="flex min-w-0 flex-col gap-0.5">
                <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- external board URL, opened in a new tab; not an internal route -->
                <a href={c.url}
                  target="_blank"
                  rel="noopener noreferrer"
                  class="truncate text-sm font-medium hover:underline"
                >
                  {c.board || hostOf(c.url)}
                </a>
                <span class="truncate text-xs text-muted-foreground">
                  {#if c.status === 'review'}
                    <span class="font-medium text-foreground">Under review</span> · not credited yet
                    · {timeAgo(c.created_at)}{surfaceLabel(c.surface)}
                  {:else}
                    {c.source} · contributed {timeAgo(c.created_at)}{surfaceLabel(c.surface)}
                  {/if}
                </span>
              </div>
            </li>
          {/each}
        </ul>
      {/if}
    </div>
  </div>
{/if}
