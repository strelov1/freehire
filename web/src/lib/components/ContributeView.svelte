<script lang="ts">
  import { invalidateAll } from '$app/navigation';
  import { api, ApiError } from '$lib/api';
  import { AsyncData } from '$lib/asyncData.svelte';
  import { currentUser, isAuthenticated } from '$lib/auth.svelte';
  import type { Contribution } from '$lib/types';
  import { Button, Input } from '$lib/ui';
  import { timeAgo } from '$lib/utils';
  import States from './States.svelte';

  let url = $state('');
  let submitting = $state(false);
  let formError = $state<string | null>(null);
  // The just-accepted contribution, shown as confirmation (and whether it opened a new board).
  let accepted = $state.raw<Contribution | null>(null);

  // The reward balance rides on the session user (page.data.user); invalidateAll() after a
  // successful contribution re-resolves it so the count updates without a manual refresh.
  const points = $derived(currentUser()?.points ?? 0);

  const canSubmit = $derived(url.trim() !== '' && !submitting);

  // Load the caller's own contributions once the session is confirmed.
  const contribData = new AsyncData<Contribution[]>([]);
  $effect(() => {
    if (isAuthenticated()) void contribData.run(() => api.listMyContributions());
  });
  const status = $derived(contribData.status);
  const contributions = $derived(contribData.value);

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    if (!canSubmit) return;
    submitting = true;
    formError = null;
    accepted = null;
    try {
      accepted = await api.submitContribution(url.trim());
      url = '';
      // Re-resolve the session user for the credited point; because the list `$effect`
      // reads page.data.user, this also re-runs it, refreshing the list in one shot.
      await invalidateAll();
    } catch (err) {
      // 422 = unsupported ATS, 409 = already held / already contributed — surface the
      // backend's specific message so the user knows which it was.
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
    <div class="flex items-start justify-between gap-4">
      <div class="flex flex-col gap-1">
        <h1 class="text-2xl font-semibold tracking-tight">Contribute a board</h1>
        <p class="text-sm text-muted-foreground">
          Found a company we don't cover yet? Paste any link from its ATS careers page — a
          vacancy or the board itself. If it's a board we don't crawl, we add it and you earn a
          point. We then pull in all of its jobs.
        </p>
      </div>
      <div
        class="shrink-0 rounded-lg border border-border bg-secondary/40 px-3 py-2 text-center"
        title="One point per new board"
      >
        <div class="text-xl font-semibold tabular-nums">{points}</div>
        <div class="text-xs text-muted-foreground">points</div>
      </div>
    </div>

    {#if accepted}
      <div class="rounded-lg border border-border bg-secondary/40 p-4 text-sm" role="status">
        Thanks — <span class="font-medium">{accepted.board}</span> ({accepted.source}) is a new
        board for us. We'll start crawling it. <span class="font-medium">+1 point.</span>
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
                  {c.board}
                </a>
                <span class="truncate text-xs text-muted-foreground">
                  {c.source} · contributed {timeAgo(c.created_at)}
                </span>
              </div>
            </li>
          {/each}
        </ul>
      {/if}
    </div>
  </div>
{/if}
