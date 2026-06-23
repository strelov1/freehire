<script lang="ts">
  import { resolve } from '$app/paths';
  import { createApiKey, listApiKeys, revokeApiKey } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import type { ApiKey, CreatedApiKey } from '$lib/types';
  import { Button, Input } from '$lib/ui';
  import { timeAgo } from '$lib/utils';
  import States from './States.svelte';

  let status = $state<'loading' | 'error' | 'ready'>('loading');
  let keys = $state.raw<ApiKey[]>([]);

  // Create form.
  const expiryOptions = [
    { label: 'No expiry', days: 0 },
    { label: '30 days', days: 30 },
    { label: '90 days', days: 90 },
    { label: '1 year', days: 365 },
  ];
  let name = $state('');
  let expiryDays = $state(0);
  let creating = $state(false);
  let formError = $state<string | null>(null);

  // The plaintext token of the just-created key — shown here exactly once, then
  // dismissed. It is never persisted client-side and never fetched again.
  let revealed = $state.raw<CreatedApiKey | null>(null);
  let copied = $state(false);

  // Show the fastest path to actually using the key: hand it to the CLI and
  // search. The raw Bearer-header call still works (noted in the page intro),
  // but the CLI is the intended client — see /cli.
  const cliExample = $derived(
    revealed
      ? `freehire auth login --token ${revealed.token}\nfreehire search "golang" --remote`
      : '',
  );

  async function load() {
    status = 'loading';
    try {
      keys = await listApiKeys();
      status = 'ready';
    } catch {
      status = 'error';
    }
  }

  // Load once the session is confirmed (the boot-time /me resolution may still be
  // in flight when the page is opened directly), mirroring MyJobsView.
  $effect(() => {
    if (isAuthenticated()) void load();
  });

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    const trimmed = name.trim();
    if (!trimmed || creating) return;
    creating = true;
    formError = null;
    copied = false;
    try {
      const expiresAt =
        expiryDays > 0 ? new Date(Date.now() + expiryDays * 86_400_000).toISOString() : undefined;
      const created = await createApiKey(trimmed, expiresAt);
      revealed = created;
      keys = [created, ...keys];
      name = '';
      expiryDays = 0;
    } catch {
      formError = 'Could not create the key. Please try again.';
    } finally {
      creating = false;
    }
  }

  async function copyToken() {
    if (!revealed) return;
    try {
      await navigator.clipboard.writeText(revealed.token);
      copied = true;
    } catch {
      copied = false;
    }
  }

  async function revoke(key: ApiKey) {
    if (!window.confirm(`Revoke "${key.name}"? Any script using it stops working immediately.`)) {
      return;
    }
    try {
      await revokeApiKey(key.id);
      keys = keys.filter((k) => k.id !== key.id);
      if (revealed?.id === key.id) revealed = null;
    } catch {
      formError = 'Could not revoke the key. Please try again.';
    }
  }

  const selectClass =
    'h-9 rounded-lg border border-input bg-transparent px-3 text-sm transition-colors focus-visible:border-ring focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50 dark:bg-input/30';
</script>

{#if !isAuthenticated()}
  <p class="py-12 text-center text-sm text-muted-foreground">
    Sign in to create and manage API keys.
  </p>
{:else}
  <div class="flex flex-col gap-6">
    <div class="flex flex-col gap-1">
      <h1 class="text-2xl font-semibold tracking-tight">API keys</h1>
      <p class="text-sm text-muted-foreground">
        Reach the API without a browser — search, open jobs, and track applications from a script.
        Use the <a href={resolve('/cli')} class="font-medium text-foreground underline-offset-4 hover:underline">freehire CLI</a>,
        or send the key directly as
        <code class="rounded bg-muted px-1 py-0.5 font-mono text-xs">Authorization: Bearer &lt;key&gt;</code>.
        See the <a href={resolve('/docs/api')} class="font-medium text-foreground underline-offset-4 hover:underline">API reference</a>
        for every endpoint and filter.
      </p>
    </div>

    {#if revealed}
      <div class="flex flex-col gap-3 rounded-lg border border-border bg-secondary/40 p-4">
        <div class="flex items-start justify-between gap-3">
          <div class="flex flex-col gap-0.5">
            <p class="text-sm font-medium">New key “{revealed.name}”</p>
            <p class="text-xs text-muted-foreground">Copy it now — it won’t be shown again.</p>
          </div>
          <button
            type="button"
            onclick={() => (revealed = null)}
            aria-label="Dismiss"
            class="text-muted-foreground transition-colors hover:text-foreground"
          >
            ✕
          </button>
        </div>
        <div class="flex items-center gap-2">
          <code class="flex-1 overflow-x-auto rounded bg-background px-3 py-2 font-mono text-sm"
            >{revealed.token}</code
          >
          <Button variant="secondary" size="sm" onclick={copyToken}>{copied ? 'Copied' : 'Copy'}</Button>
        </div>
        <pre
          class="overflow-x-auto rounded bg-background px-3 py-2 font-mono text-xs text-muted-foreground">{cliExample}</pre>
        <p class="text-xs text-muted-foreground">
          New to the CLI? See the <a href={resolve('/cli')} class="font-medium text-foreground underline-offset-4 hover:underline">command reference</a>.
        </p>
      </div>
    {/if}

    <form
      onsubmit={submit}
      class="flex flex-col gap-3 rounded-lg border border-border p-4 sm:flex-row sm:items-end"
    >
      <label class="flex flex-1 flex-col gap-1">
        <span class="text-sm font-medium">Name</span>
        <Input bind:value={name} placeholder="e.g. CI bot" maxlength={100} class="w-full" />
      </label>
      <label class="flex flex-col gap-1">
        <span class="text-sm font-medium">Expiry</span>
        <select bind:value={expiryDays} class={selectClass}>
          {#each expiryOptions as opt (opt.days)}
            <option value={opt.days}>{opt.label}</option>
          {/each}
        </select>
      </label>
      <Button variant="primary" type="submit" disabled={!name.trim() || creating}>
        {creating ? 'Creating…' : 'Create key'}
      </Button>
    </form>

    {#if formError}
      <p class="text-sm text-destructive">{formError}</p>
    {/if}

    {#if status === 'loading'}
      <States state="loading" />
    {:else if status === 'error'}
      <States state="error" message="Couldn't load your API keys." />
    {:else if keys.length === 0}
      <States state="empty" message="No API keys yet. Create one above to use the API from a script." />
    {:else}
      <ul class="flex flex-col divide-y divide-border rounded-lg border border-border">
        {#each keys as key (key.id)}
          <li class="flex items-center justify-between gap-3 px-4 py-3">
            <div class="flex min-w-0 flex-col gap-0.5">
              <span class="truncate text-sm font-medium">{key.name}</span>
              <span class="font-mono text-xs text-muted-foreground">{key.token_prefix}…</span>
              <span class="text-xs text-muted-foreground">
                Created {timeAgo(key.created_at)} ·
                {key.last_used_at ? `last used ${timeAgo(key.last_used_at)}` : 'never used'}
                {#if key.expires_at}· expires {timeAgo(key.expires_at)}{/if}
              </span>
            </div>
            <Button variant="ghost" size="sm" onclick={() => revoke(key)}>Revoke</Button>
          </li>
        {/each}
      </ul>
    {/if}
  </div>
{/if}
