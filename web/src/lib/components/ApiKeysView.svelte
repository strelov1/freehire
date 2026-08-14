<script lang="ts">
  import { resolve } from '$app/paths';
  import { api, ApiError } from '$lib/api';
  import { AsyncData } from '$lib/asyncData.svelte';
  import { currentUser, isAuthenticated } from '$lib/auth.svelte';
  import { beginProviderReauthentication } from '$lib/recentAuth';
  import type { ApiKey, CreatedApiKey } from '$lib/types';
  import { Button, ConfirmDialog, Input } from '$lib/ui';
  import { timeAgo } from '$lib/utils';
  import States from './States.svelte';

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
  let confirmationPassword = $state('');
  let recentAuthRequired = $state(false);
  const user = $derived(currentUser());
  const hasPassword = $derived(user?.has_password ?? false);
  const identitiesData = new AsyncData<string[]>([]);

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

  // Load once the session is confirmed (the boot-time /me resolution may still be
  // in flight when the page is opened directly), mirroring MyJobsView.
  const keysData = new AsyncData<ApiKey[]>([]);
  $effect(() => {
    if (isAuthenticated()) void keysData.run(() => api.listApiKeys());
  });
  $effect(() => {
    if (isAuthenticated() && !hasPassword && recentAuthRequired) {
      void identitiesData.run(async () =>
        (await api.connectedIdentities()).identities
          .filter((i) => i.status === 'active')
          .map((i) => i.provider),
      );
    }
  });
  const status = $derived(keysData.status);
  const keys = $derived(keysData.value);

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    const trimmed = name.trim();
    if (!trimmed || creating) return;
    creating = true;
    formError = null;
    copied = false;
    try {
      if (hasPassword) {
        if (!confirmationPassword) {
          formError = 'Enter your password to confirm this security change.';
          return;
        }
        await api.reauthenticatePassword(confirmationPassword);
      }
      const expiresAt =
        expiryDays > 0 ? new Date(Date.now() + expiryDays * 86_400_000).toISOString() : undefined;
      const created = await api.createApiKey(trimmed, expiresAt);
      revealed = created;
      keysData.value = [created, ...keysData.value];
      name = '';
      expiryDays = 0;
      confirmationPassword = '';
    } catch (error) {
      if (error instanceof ApiError && error.status === 428) {
        formError = 'Confirm your identity before creating a key.';
        recentAuthRequired = true;
      } else if (error instanceof ApiError && error.status === 401 && hasPassword) {
        formError = 'That password is not right.';
      } else {
        formError = 'Could not create the key. Please try again.';
      }
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

  let revokeTarget = $state<ApiKey | null>(null);
  let confirmRevokeOpen = $state(false);

  function requestRevoke(key: ApiKey) {
    revokeTarget = key;
    confirmRevokeOpen = true;
  }

  async function revoke() {
    const key = revokeTarget;
    if (!key) return;
    if (hasPassword && !confirmationPassword) {
      formError = 'Enter your password to confirm this security change.';
      throw new Error(formError);
    }
    try {
      if (hasPassword) await api.reauthenticatePassword(confirmationPassword);
      await api.revokeApiKey(key.id);
      keysData.value = keysData.value.filter((k) => k.id !== key.id);
      if (revealed?.id === key.id) revealed = null;
      confirmationPassword = '';
    } catch (error) {
      if (error instanceof ApiError && error.status === 428) {
        formError = 'Confirm your identity before revoking a key.';
        recentAuthRequired = true;
      } else if (error instanceof ApiError && error.status === 401 && hasPassword) {
        formError = 'That password is not right.';
      } else {
        formError = 'Could not revoke the key. Please try again.';
      }
      throw new Error(formError, { cause: error });
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
      {#if hasPassword}
        <label class="flex flex-col gap-1">
          <span class="text-sm font-medium">Confirm password</span>
          <Input
            type="password"
            bind:value={confirmationPassword}
            autocomplete="current-password"
            class="w-full"
          />
        </label>
      {/if}
      <Button variant="primary" type="submit" disabled={!name.trim() || creating}>
        {creating ? 'Creating…' : 'Create key'}
      </Button>
    </form>

    {#if !hasPassword && recentAuthRequired}
      <div class="rounded-lg border border-border p-4">
        <p class="mb-3 text-sm text-muted-foreground">
          Confirm your identity with a connected provider before creating or revoking a key.
        </p>
        {#if identitiesData.status === 'loading'}
          <p class="text-sm text-muted-foreground">Loading connected providers…</p>
        {:else if identitiesData.status === 'error'}
          <p class="text-sm text-destructive">Could not load connected providers.</p>
        {:else}
          <div class="flex flex-wrap gap-2">
            {#each identitiesData.value as provider (provider)}
              <Button
                variant="outline"
                size="sm"
                onclick={() => beginProviderReauthentication(provider, '/my/api-keys')}
              >Confirm with {provider}</Button>
            {/each}
          </div>
        {/if}
      </div>
    {/if}

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
            <Button variant="ghost" size="sm" onclick={() => requestRevoke(key)}>Revoke</Button>
          </li>
        {/each}
      </ul>
    {/if}
  </div>

  <ConfirmDialog
    bind:open={confirmRevokeOpen}
    title={`Revoke "${revokeTarget?.name ?? ''}"?`}
    description="Any script using it stops working immediately."
    confirmLabel="Revoke"
    variant="destructive"
    onConfirm={revoke}
  />
{/if}
