<script lang="ts">
  import { resolve } from '$app/paths';
  import { X } from '@lucide/svelte';
  import { api } from '#lib/api.js';
  import { AsyncData } from '#lib/asyncData.svelte.js';
  import { isAuthenticated } from '#lib/auth.svelte.js';
  import { consumeReauthDraft } from '#lib/recentAuth.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { format, t } from '#lib/i18n/t.js';
  import { announce } from '#lib/announce.svelte.js';
  import type { ApiKey, CreatedApiKey } from '#lib/types.js';
  import { Button, ConfirmDialog } from '#lib/ui/index.js';
  import { timeAgo } from '#lib/utils.js';
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
  let copyFailed = $state(false);
  // Dismissing destroys the only copy of the secret there will ever be, so an unconfirmed
  // dismissal asks first — unless the key was already copied, which is the answer.
  let confirmingDismiss = $state(false);
  let tokenEl = $state<HTMLElement>();
  // Where focus goes when the confirmation ends: back to the ✕ that asked, or — once the
  // panel holding both is gone — to "Create key", the nearest control that is still there.
  let dismissButton = $state<HTMLButtonElement>();
  let createButton = $state<HTMLElement>();
  const confirmTextId = $props.id();

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
    if (isAuthenticated()) loadKeys();
  });
  const status = $derived(keysData.status);
  const keys = $derived(keysData.value);

  function loadKeys(): void {
    void keysData.run(() => api.listApiKeys());
  }

  function onCreated(key: CreatedApiKey): void {
    revealed = key;
    copied = false;
    copyFailed = false;
    confirmingDismiss = false;
    keysData.value = [key, ...keysData.value];
  }

  async function copyToken(): Promise<void> {
    if (!revealed) return;
    try {
      await navigator.clipboard.writeText(revealed.token);
      copied = true;
      copyFailed = false;
      announce(s.reveal.copied);
    } catch {
      // A denied or missing clipboard must not look like nothing happened: say so, and do
      // the first half of the manual copy for them by selecting the token.
      copied = false;
      copyFailed = true;
      announce(s.reveal.copyFailed, 'assertive');
      if (tokenEl) window.getSelection()?.selectAllChildren(tokenEl);
    }
  }

  function requestDismiss(): void {
    if (copied) {
      dismiss();
      return;
    }
    // The confirmation's "Yes" takes focus as it mounts (see its attachment).
    confirmingDismiss = true;
  }

  function dismiss(): void {
    revealed = null;
    createButton?.focus();
  }

  function keepRevealed(): void {
    confirmingDismiss = false;
    dismissButton?.focus();
  }

  // Escape answers "keep it": the safe choice, the same one leaving the question unanswered
  // makes. On the bar's two buttons, which are the only places focus can be inside it.
  function cancelOnEscape(e: KeyboardEvent): void {
    if (e.key === 'Escape') keepRevealed();
  }

  let revokeTarget = $state<ApiKey | null>(null);
  let confirmRevokeOpen = $state(false);
  let revokePassword = $state('');
  let revokeIdentity = $state<ConfirmIdentity | null>(null);

  // Reopen the revocation the member left to confirm. The key ID travels rather than the
  // key itself: the list is re-fetched on mount, so the row is looked up fresh — a stored
  // copy could be of a key that has since been revoked from another tab.
  $effect(() => {
    const pending = consumeReauthDraft('revoke-api-key');
    if (!pending) return;
    pendingRevokeId = pending.keyId;
  });
  // Held until the list arrives, since the dialog names the key and the fetch may still be
  // in flight when the draft is read.
  let pendingRevokeId = $state<number | null>(null);
  $effect(() => {
    if (pendingRevokeId === null) return;
    const key = keysData.value.find((k) => k.id === pendingRevokeId);
    if (!key) return;
    pendingRevokeId = null;
    requestRevoke(key);
  });

  function requestRevoke(key: ApiKey): void {
    revokeTarget = key;
    revokePassword = '';
    confirmRevokeOpen = true;
  }

  async function revoke(): Promise<void> {
    const key = revokeTarget;
    if (!key) return;
    try {
      if (!(await revokeIdentity?.prove())) throw new Error(s.errors.confirmFirst);
      await api.revokeApiKey(key.id);
      keysData.value = keysData.value.filter((k) => k.id !== key.id);
      if (revealed?.id === key.id) revealed = null;
      revokePassword = '';
    } catch (error) {
      // ConfirmDialog shows a thrown message in place and holds itself open, which is what
      // keeps the confirmation the member needs on screen beside the failure that asked
      // for it. Identity refusals are worded by the component that asked for the proof.
      throw new Error(revokeIdentity?.handleRefusal(error) ?? s.errors.revokeFailed, {
        cause: error,
      });
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
      <Button
        variant="primary"
        onclick={() => (createOpen = true)}
        {@attach (el: HTMLElement) => {
          createButton = el;
        }}
      >
        {s.form.create}
      </Button>
    </div>

    {#if revealed}
      <div class="flex flex-col gap-3 rounded-lg border border-border bg-secondary/40 p-4">
        <div class="flex items-start justify-between gap-3">
          <div class="flex flex-col gap-0.5">
            <p class="text-sm font-medium">{s.reveal.newKeyPrefix} “{revealed.name}”</p>
            <p class="text-xs text-muted-foreground">{s.reveal.copyNow}</p>
          </div>
          <button
            bind:this={dismissButton}
            type="button"
            onclick={requestDismiss}
            aria-label={s.reveal.dismiss}
            class="-m-1.5 flex size-8 shrink-0 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
          >
            <X class="size-4" aria-hidden="true" />
          </button>
        </div>
        {#if confirmingDismiss}
          <div
            role="group"
            aria-labelledby={confirmTextId}
            class="flex flex-wrap items-center gap-2 rounded-md bg-warning/10 px-3 py-2 text-sm"
          >
            <p id={confirmTextId} class="flex-1">{s.reveal.dismissConfirm}</p>
            <Button
              variant="secondary"
              size="sm"
              onclick={dismiss}
              onkeydown={cancelOnEscape}
              {@attach (el: HTMLElement) => el.focus()}
            >
              {s.reveal.dismissConfirmYes}
            </Button>
            <Button variant="ghost" size="sm" onclick={keepRevealed} onkeydown={cancelOnEscape}>
              {s.reveal.dismissConfirmNo}
            </Button>
          </div>
        {/if}
        <div class="flex items-center gap-2">
          <code bind:this={tokenEl} class="flex-1 overflow-x-auto rounded bg-background px-3 py-2 font-mono text-sm"
            >{revealed.token}</code
          >
          <Button variant="secondary" size="sm" onclick={copyToken}>
            {copied ? s.reveal.copied : s.reveal.copy}
          </Button>
        </div>
        {#if copyFailed}
          <p class="text-xs text-destructive">{s.reveal.copyFailed}</p>
        {/if}
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
      <States state="error" message={s.list.loadError} onRetry={loadKeys} />
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
                {timeAgo(key.created_at, locale())} ·
                {key.last_used_at
                  ? `${s.list.lastUsedPrefix} ${timeAgo(key.last_used_at, locale())}`
                  : s.list.neverUsed}
                {#if key.expires_at}· {s.list.expiresPrefix} {timeAgo(key.expires_at, locale())}{/if}
              </span>
            </div>
            <Button
              variant="ghost"
              size="sm"
              aria-label={format(s.list.revokeAria, { name: key.name })}
              onclick={() => requestRevoke(key)}
            >
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
         existed on the page behind its own backdrop. -->
    <ConfirmIdentity
      bind:this={revokeIdentity}
      bind:password={revokePassword}
      returnTo="/my/api-keys"
      prompt={s.revokeDialog.confirmPrompt}
      draft={() => ({ surface: 'revoke-api-key', keyId: revokeTarget?.id ?? 0 })}
      active={confirmRevokeOpen}
    />
  </ConfirmDialog>
{/if}
