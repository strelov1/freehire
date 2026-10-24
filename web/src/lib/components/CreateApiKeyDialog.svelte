<script lang="ts">
  import { ApiError, api } from '#lib/api.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { consumeReauthDraft } from '#lib/recentAuth.js';
  import type { CreatedApiKey } from '#lib/types.js';
  import { Button, Dialog, Input } from '#lib/ui/index.js';
  import ConfirmIdentity from './ConfirmIdentity.svelte';
  import { messages } from './CreateApiKeyDialog.messages';

  // Minting a key, with the identity check as a step of the form rather than as a red error
  // raised after the server refuses. The check lives INSIDE the dialog because the dialog is
  // modal: an input on the page behind it is not reachable, which is exactly how the revoke
  // path came to demand a password nobody could type.
  let {
    open = $bindable(false),
    onCreated,
  }: {
    open?: boolean;
    /** Handed the created key so the page can list it and reveal the token once. */
    onCreated: (key: CreatedApiKey) => void;
  } = $props();

  const s = $derived(t(messages, locale()));
  const expiryOptions = $derived([
    { label: s.expiryNever, days: 0 },
    { label: s.expiry30, days: 30 },
    { label: s.expiry90, days: 90 },
    { label: s.expiry365, days: 365 },
  ]);
  // "" (full) is the default and is never sent to the server — unchanged from
  // before scope existed. "embed" is the only other value a client may request
  // (see api.createApiKey's doc and the server's own scope check).
  const scopeOptions = $derived([
    { label: s.scopeFull, value: '' },
    { label: s.scopeEmbed, value: 'embed' },
  ]);

  let name = $state('');
  let days = $state(0);
  let scope = $state('');
  let password = $state('');
  let creating = $state(false);
  let error = $state<string | null>(null);
  let identity = $state<ConfirmIdentity | null>(null);

  // Reopen where the member left off. Confirming through a provider is a full-page
  // navigation, so without this they come back to an empty form with no sign that anything
  // worked — which is the complaint that started this change.
  //
  // Reopening only; the key is NOT created here. The member left to prove who they are, not
  // to place an order, and an action that completes itself while they were away is a
  // surprise on a surface whose whole subject is deliberateness.
  $effect(() => {
    const draft = consumeReauthDraft('create-api-key');
    if (!draft) return;
    name = draft.name;
    days = draft.days;
    scope = draft.scope;
    open = true;
  });

  // Not reset on close: a member who closes by mistake should not lose what they typed. It
  // is cleared on success instead, where there is nothing left to lose.
  function reset(): void {
    name = '';
    days = 0;
    scope = '';
    password = '';
    error = null;
  }

  async function submit(e: SubmitEvent): Promise<void> {
    e.preventDefault();
    const trimmed = name.trim();
    if (creating) return;
    if (!trimmed) {
      error = s.errors.nameRequired;
      return;
    }
    creating = true;
    error = null;
    try {
      // Holds a proof or buys one with the typed password — the component decides which,
      // and says so itself when the member has supplied neither.
      if (!(await identity?.prove())) return;
      const expiresAt =
        days > 0 ? new Date(Date.now() + days * 86_400_000).toISOString() : undefined;
      onCreated(await api.createApiKey(trimmed, expiresAt, scope || undefined));
      reset();
      open = false;
    } catch (e) {
      // Identity refusals are worded once, by the component that asked for the proof;
      // anything else is this dialog's own to explain.
      error = identity?.handleRefusal(e) ?? messageFor(e);
    } finally {
      creating = false;
    }
  }

  function messageFor(e: unknown): string {
    if (!(e instanceof ApiError)) return s.errors.createFailed;
    return e.message || s.errors.createFailed;
  }
</script>

<Dialog bind:open title={s.title} dismissible={!creating} class="sm:max-w-md">
  <!-- `{#if open}` because Dialog renders its children unconditionally, and this form's own
       inputs would otherwise sit in the DOM of a page nobody opened it on. -->
  {#if open}
    <form onsubmit={submit} class="flex flex-col gap-4">
      <label class="flex flex-col gap-1">
        <span class="text-sm font-medium">{s.nameLabel}</span>
        <Input
          bind:value={name}
          placeholder={s.namePlaceholder}
          maxlength={100}
          disabled={creating}
        />
      </label>

      <label class="flex flex-col gap-1">
        <span class="text-sm font-medium">{s.expiryLabel}</span>
        <select
          bind:value={days}
          disabled={creating}
          class="h-9 rounded-lg border border-input bg-transparent px-3 text-sm transition-colors focus-visible:border-ring focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring dark:bg-input/30"
        >
          {#each expiryOptions as opt (opt.days)}
            <option value={opt.days}>{opt.label}</option>
          {/each}
        </select>
      </label>

      <div class="flex flex-col gap-1">
        <label class="flex flex-col gap-1">
          <span class="text-sm font-medium">{s.scopeLabel}</span>
          <select
            bind:value={scope}
            disabled={creating}
            class="h-9 rounded-lg border border-input bg-transparent px-3 text-sm transition-colors focus-visible:border-ring focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring dark:bg-input/30"
          >
            {#each scopeOptions as opt (opt.value)}
              <option value={opt.value}>{opt.label}</option>
            {/each}
          </select>
        </label>
        <!-- Outside the label on purpose: a wrapper <label>'s accessible name is every
             text node inside it, so a hint sharing the label would get concatenated
             onto "Scope" instead of staying its own readable sentence. -->
        <span class="text-xs text-muted-foreground">{s.scopeHint}</span>
      </div>

      <ConfirmIdentity
        bind:this={identity}
        bind:password
        returnTo="/my/api-keys"
        draft={() => ({ surface: 'create-api-key', name, days, scope })}
        prompt={s.confirmPrompt}
        active={open}
        disabled={creating}
      />

      {#if error}
        <p class="text-sm text-destructive">{error}</p>
      {/if}

      <div class="flex items-center justify-end gap-2">
        <Button variant="ghost" size="sm" disabled={creating} onclick={() => (open = false)}>
          {s.cancel}
        </Button>
        <Button variant="primary" size="sm" type="submit" disabled={creating}>
          {creating ? s.creating : s.create}
        </Button>
      </div>
    </form>
  {/if}
</Dialog>
