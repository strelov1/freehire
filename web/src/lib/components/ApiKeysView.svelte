<script lang="ts">
  import { resolve } from '$app/paths';
  import { api, ApiError } from '$lib/api';
  import { AsyncData } from '$lib/asyncData.svelte';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { locale } from '$lib/i18n/currentLocale.svelte';
  import { t } from '$lib/i18n/t';
  import type { ApiKey, CreatedApiKey } from '$lib/types';
  import { Button, ConfirmDialog } from '$lib/ui';
  import { timeAgo } from '$lib/utils';
  import { messages } from './ApiKeysView.messages';
  import ConfirmIdentity from './ConfirmIdentity.svelte';
  import CreateApiKeyDialog from './CreateApiKeyDialog.svelte';
  import States from './States.svelte';

  // The page is a list and two dialogs. It used to be a list and a permanently open create
  // form, and the identity check was spread across both: the password input lived in that
  // form while the revoke dialog demanded a password — from behind a modal backdrop, where
  // no input is reachable. Each dialog now carries its own confirmation.
  const s = $derived(t(messages, locale()));

  let createOpen = $state(false);

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
  const status = $derived(keysData.status);
  const keys = $derived(keysData.value);

  function onCreated(key: CreatedApiKey): void {
    revealed = key;
    copied = false;
    keysData.value = [key, ...keysData.value];
  }

  async function copyToken(): Promise<void> {
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
  let revokePassword = $state('');
  let revokeIdentity = $state<ConfirmIdentity | null>(null);

  function requestRevoke(key: ApiKey): void {
    revokeTarget = key;
    revokePassword = '';
    confirmRevokeOpen = true;
  }

  async function revoke(): Promise<void> {
    const key = revokeTarget;
    if (!key) return;
    try {
      await revokeIdentity?.prove();
      await api.revokeApiKey(key.id);
      keysData.value = keysData.value.filter((k) => k.id !== key.id);
      if (revealed?.id === key.id) revealed = null;
      revokePassword = '';
    } catch (error) {
      // ConfirmDialog shows a thrown message in place and holds itself open, which is what
      // keeps the confirmation the member needs on screen with the failure that asked
      // for it.
      if (error instanceof ApiError && error.status === 428) {
        revokeIdentity?.refused();
        throw new Error(s.errors.reauthBeforeRevoke, { cause: error });
      }
      const message =
        error instanceof ApiError && error.status === 401 && !revokeIdentity?.isConfirmed()
          ? s.errors.wrongPassword
          : s.errors.revokeFailed;
      throw new Error(message, { cause: error });
    }
  }
</script>

{#if !isAuthenticated()}
  <p class="py-12 text-center text-sm text-muted-foreground">
    {s.signedOut}
  </p>
{:else}
  <div class="flex flex-col gap-6">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div class="flex flex-col gap-1">
        <h1 class="text-2xl font-semibold tracking-tight">{s.title}</h1>
        <p class="text-sm text-muted-foreground">
          {s.intro.lead}
          <a href={resolve('/cli')} class="font-medium text-foreground underline-offset-4 hover:underline">{s.intro.cliLink}</a>{s.intro.orSendDirectly}
          <code class="rounded bg-muted px-1 py-0.5 font-mono text-xs">Authorization: Bearer &lt;key&gt;</code>{s.intro.seeThe}
          <a href={resolve('/docs/api')} class="font-medium text-foreground underline-offset-4 hover:underline">{s.intro.apiReferenceLink}</a>
          {s.intro.apiReferenceTail}
        </p>
      </div>
      <Button variant="primary" onclick={() => (createOpen = true)}>{s.form.create}</Button>
    </div>

    {#if revealed}
      <div class="flex flex-col gap-3 rounded-lg border border-border bg-secondary/40 p-4">
        <div class="flex items-start justify-between gap-3">
          <div class="flex flex-col gap-0.5">
            <p class="text-sm font-medium">{s.reveal.newKeyPrefix} “{revealed.name}”</p>
            <p class="text-xs text-muted-foreground">{s.reveal.copyNow}</p>
          </div>
          <button
            type="button"
            onclick={() => (revealed = null)}
            aria-label={s.reveal.dismiss}
            class="text-muted-foreground transition-colors hover:text-foreground"
          >
            ✕
          </button>
        </div>
        <div class="flex items-center gap-2">
          <code class="flex-1 overflow-x-auto rounded bg-background px-3 py-2 font-mono text-sm"
            >{revealed.token}</code
          >
          <Button variant="secondary" size="sm" onclick={copyToken}>
            {copied ? s.reveal.copied : s.reveal.copy}
          </Button>
        </div>
        <pre
          class="overflow-x-auto rounded bg-background px-3 py-2 font-mono text-xs text-muted-foreground">{cliExample}</pre>
        <p class="text-xs text-muted-foreground">
          {s.reveal.newToCli}
          <a href={resolve('/cli')} class="font-medium text-foreground underline-offset-4 hover:underline">{s.reveal.commandReferenceLink}</a>.
        </p>
      </div>
    {/if}

    {#if status === 'loading'}
      <States state="loading" />
    {:else if status === 'error'}
      <States state="error" message={s.list.loadError} />
    {:else if keys.length === 0}
      <States state="empty" message={s.list.empty} />
    {:else}
      <ul class="flex flex-col divide-y divide-border rounded-lg border border-border">
        {#each keys as key (key.id)}
          <li class="flex items-center justify-between gap-3 px-4 py-3">
            <div class="flex min-w-0 flex-col gap-0.5">
              <span class="truncate text-sm font-medium">{key.name}</span>
              <span class="font-mono text-xs text-muted-foreground">{key.token_prefix}…</span>
              <span class="text-xs text-muted-foreground">
                {s.list.createdPrefix}
                {timeAgo(key.created_at)} ·
                {key.last_used_at
                  ? `${s.list.lastUsedPrefix} ${timeAgo(key.last_used_at)}`
                  : s.list.neverUsed}
                {#if key.expires_at}· {s.list.expiresPrefix} {timeAgo(key.expires_at)}{/if}
              </span>
            </div>
            <Button variant="ghost" size="sm" onclick={() => requestRevoke(key)}>
              {s.list.revoke}
            </Button>
          </li>
        {/each}
      </ul>
    {/if}
  </div>

  <CreateApiKeyDialog bind:open={createOpen} {onCreated} />

  <ConfirmDialog
    bind:open={confirmRevokeOpen}
    title={`${s.revokeDialog.titlePrefix} "${revokeTarget?.name ?? ''}"${s.revokeDialog.titleSuffix}`}
    description={s.revokeDialog.description}
    confirmLabel={s.revokeDialog.confirmLabel}
    variant="destructive"
    onConfirm={revoke}
  >
    <!-- The fix for the original report. ConfirmDialog has always accepted `children`;
         this page simply never passed any, so the dialog asked for a password that only
         existed on the page behind its own backdrop. `{#if}` because Dialog renders its
         children whether or not it is open. -->
    {#if confirmRevokeOpen}
      <ConfirmIdentity
        bind:this={revokeIdentity}
        bind:password={revokePassword}
        returnTo="/my/api-keys"
        prompt={s.revokeDialog.confirmPrompt}
      />
    {/if}
  </ConfirmDialog>
{/if}
