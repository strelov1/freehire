<script lang="ts">
  import { api, ApiError } from '$lib/api';
  import { AsyncData } from '$lib/asyncData.svelte';
  import { isAuthenticated } from '$lib/auth.svelte';
  import type { WebhookConfig } from '$lib/types';
  import { Button, ConfirmDialog, Input } from '$lib/ui';
  import { timeAgo } from '$lib/utils';
  import States from './States.svelte';

  // Load once the session is confirmed, mirroring ApiKeysView.
  const webhookData = new AsyncData<WebhookConfig | null>(null);
  $effect(() => {
    if (isAuthenticated()) void webhookData.run(() => api.getWebhook());
  });
  const status = $derived(webhookData.status);
  const webhook = $derived(webhookData.value);

  let url = $state('');
  let saving = $state(false);
  let formError = $state<string | null>(null);

  $effect(() => {
    if (webhook && !url) url = webhook.url;
  });

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    const trimmed = url.trim();
    if (!trimmed || saving) return;
    saving = true;
    formError = null;
    try {
      webhookData.value = await api.createOrUpdateWebhook(trimmed);
    } catch (error) {
      formError =
        error instanceof ApiError && error.status === 400
          ? 'Enter a valid http:// or https:// URL.'
          : 'Could not save the webhook. Please try again.';
    } finally {
      saving = false;
    }
  }

  async function toggleEnabled() {
    if (!webhook) return;
    try {
      webhookData.value = await api.setWebhookEnabled(!webhook.enabled);
    } catch {
      formError = 'Could not update the webhook. Please try again.';
    }
  }

  let confirmDeleteOpen = $state(false);

  async function remove() {
    try {
      await api.deleteWebhook();
      webhookData.value = null;
      url = '';
    } catch (error) {
      formError = 'Could not delete the webhook. Please try again.';
      throw new Error(formError, { cause: error });
    }
  }
</script>

{#if !isAuthenticated()}
  <p class="py-12 text-center text-sm text-muted-foreground">Sign in to configure a webhook.</p>
{:else}
  <div class="flex flex-col gap-6">
    <div class="flex flex-col gap-1">
      <h1 class="text-2xl font-semibold tracking-tight">Webhook</h1>
      <p class="text-sm text-muted-foreground">
        Get an HTTP POST whenever one of your saved searches finds a new match —
        alongside or instead of email/Telegram. Turn it on for a saved search from its
        alert settings once a destination is configured here.
      </p>
    </div>

    {#if status === 'loading'}
      <States state="loading" />
    {:else}
      <form
        onsubmit={submit}
        class="flex flex-col gap-3 rounded-lg border border-border p-4 sm:flex-row sm:items-end"
      >
        <label class="flex flex-1 flex-col gap-1">
          <span class="text-sm font-medium">URL</span>
          <Input
            bind:value={url}
            type="url"
            placeholder="https://example.com/freehire-hook"
            class="w-full"
          />
        </label>
        <Button variant="primary" type="submit" disabled={!url.trim() || saving}>
          {saving ? 'Saving…' : webhook ? 'Save' : 'Create webhook'}
        </Button>
      </form>

      {#if formError}
        <p class="text-sm text-destructive">{formError}</p>
      {/if}

      {#if webhook}
        <div class="flex items-center justify-between gap-3 rounded-lg border border-border px-4 py-3">
          <div class="flex min-w-0 flex-col gap-0.5">
            <span class="truncate font-mono text-sm">{webhook.url}</span>
            <span class="text-xs text-muted-foreground">
              {#if webhook.enabled}
                Enabled · created {timeAgo(webhook.created_at)}
                {#if webhook.last_success_at}· last delivered {timeAgo(webhook.last_success_at)}{/if}
              {:else}
                Disabled
                {#if webhook.disabled_at}· since {timeAgo(webhook.disabled_at)}{/if}
              {/if}
            </span>
          </div>
          <div class="flex shrink-0 items-center gap-2">
            <Button variant="outline" size="sm" onclick={toggleEnabled}>
              {webhook.enabled ? 'Disable' : 'Enable'}
            </Button>
            <Button variant="ghost" size="sm" onclick={() => (confirmDeleteOpen = true)}
              >Delete</Button
            >
          </div>
        </div>
      {/if}
    {/if}
  </div>

  <ConfirmDialog
    bind:open={confirmDeleteOpen}
    title="Delete webhook?"
    description="Saved searches subscribed to this channel stop delivering immediately."
    confirmLabel="Delete"
    variant="destructive"
    onConfirm={remove}
  />
{/if}
