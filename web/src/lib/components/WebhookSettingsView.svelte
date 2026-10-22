<script lang="ts">
  import { api, ApiError } from '#lib/api.js';
  import { AsyncData } from '#lib/asyncData.svelte.js';
  import { isAuthenticated } from '#lib/auth.svelte.js';
  import type { WebhookConfig } from '#lib/types.js';
  import { Button, ConfirmDialog, Input } from '#lib/ui/index.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { messages } from './WebhookSettingsView.messages';
  import { timeAgo } from '#lib/utils.js';
  import States from './States.svelte';

  const s = $derived(t(messages, locale()));

  // Load once the session is confirmed, mirroring ApiKeysView.
  const webhookData = new AsyncData<WebhookConfig | null>(null);
  $effect(() => {
    if (isAuthenticated()) void webhookData.run(() => api.getWebhook());
  });
  const status = $derived(webhookData.status);
  const webhook = $derived(webhookData.value);

  let url = $state('');
  let saving = $state(false);
  // A key into the catalog, not the message itself — the text is derived from `s`
  // below so an already-shown error follows a later locale change instead of
  // freezing in whatever locale was resolved when it was set.
  let formErrorKind = $state<
    'invalidUrl' | 'saveFailed' | 'updateFailed' | 'deleteFailed' | 'testFailed' | null
  >(null);
  // What the destination answered when a test delivery failed — the server's
  // own words ("destination responded 404"), shown after the localized lead-in.
  let testFailure = $state('');
  const formError = $derived.by(() => {
    if (formErrorKind === 'testFailed') return `${s.testFailed} ${testFailure}`;
    return formErrorKind ? s[formErrorKind] : null;
  });
  let testing = $state(false);
  let testPassed = $state(false);

  // A 422 from save or test means the destination refused the sample
  // delivery; its message carries the reason after a fixed server prefix.
  function testFailureOf(error: unknown): string | null {
    if (!(error instanceof ApiError) || error.status !== 422) return null;
    return error.message.replace(/^webhook test failed:\s*/, '');
  }

  function showError(kind: NonNullable<typeof formErrorKind>, error: unknown) {
    const failure = testFailureOf(error);
    if (failure !== null) {
      testFailure = failure;
      formErrorKind = 'testFailed';
    } else {
      formErrorKind = kind;
    }
  }

  $effect(() => {
    if (webhook && !url) url = webhook.url;
  });

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    const trimmed = url.trim();
    if (!trimmed || saving) return;
    saving = true;
    formErrorKind = null;
    testPassed = false;
    try {
      webhookData.value = await api.createOrUpdateWebhook(trimmed);
    } catch (error) {
      showError(error instanceof ApiError && error.status === 400 ? 'invalidUrl' : 'saveFailed', error);
    } finally {
      saving = false;
    }
  }

  async function sendTest() {
    if (!webhook || testing) return;
    testing = true;
    formErrorKind = null;
    testPassed = false;
    try {
      webhookData.value = await api.testWebhook();
      testPassed = true;
    } catch (error) {
      showError('updateFailed', error);
    } finally {
      testing = false;
    }
  }

  async function toggleEnabled() {
    if (!webhook) return;
    try {
      webhookData.value = await api.setWebhookEnabled(!webhook.enabled);
    } catch {
      formErrorKind = 'updateFailed';
    }
  }

  let confirmDeleteOpen = $state(false);

  async function remove() {
    try {
      await api.deleteWebhook();
      webhookData.value = null;
      url = '';
    } catch (error) {
      formErrorKind = 'deleteFailed';
      throw new Error(s.deleteFailed, { cause: error });
    }
  }
</script>

{#if !isAuthenticated()}
  <p class="py-12 text-center text-sm text-muted-foreground">{s.signInPrompt}</p>
{:else}
  <div class="flex flex-col gap-6">
    <div class="flex flex-col gap-1">
      <h1 class="text-2xl font-semibold tracking-tight">{s.heading}</h1>
      <p class="text-sm text-muted-foreground">
        {s.description}
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
          <span class="text-sm font-medium">{s.urlLabel}</span>
          <Input
            bind:value={url}
            type="url"
            placeholder={s.urlPlaceholder}
            class="w-full"
          />
        </label>
        <Button variant="primary" type="submit" disabled={!url.trim() || saving}>
          {saving ? s.saving : webhook ? s.save : s.create}
        </Button>
      </form>

      {#if formError}
        <p class="text-sm text-destructive">{formError}</p>
      {:else if testPassed}
        <p class="text-sm text-muted-foreground">{s.testPassed}</p>
      {/if}

      {#if webhook}
        <div class="flex items-center justify-between gap-3 rounded-lg border border-border px-4 py-3">
          <div class="flex min-w-0 flex-col gap-0.5">
            <span class="truncate font-mono text-sm">{webhook.url}</span>
            <span class="text-xs text-muted-foreground">
              {#if webhook.enabled}
                {s.enabledPrefix} · {s.createdPrefix} {timeAgo(webhook.created_at, locale())}
                {#if webhook.last_success_at}· {s.lastDeliveredPrefix} {timeAgo(webhook.last_success_at, locale())}{/if}
              {:else}
                {s.disabled}
                {#if webhook.disabled_at}· {s.sincePrefix} {timeAgo(webhook.disabled_at, locale())}{/if}
              {/if}
            </span>
          </div>
          <div class="flex shrink-0 items-center gap-2">
            <Button variant="outline" size="sm" onclick={sendTest} disabled={testing}>
              {testing ? s.testing : s.test}
            </Button>
            <Button variant="outline" size="sm" onclick={toggleEnabled}>
              {webhook.enabled ? s.disable : s.enable}
            </Button>
            <Button variant="ghost" size="sm" onclick={() => (confirmDeleteOpen = true)}
              >{s.delete}</Button
            >
          </div>
        </div>
      {/if}
    {/if}
  </div>

  <ConfirmDialog
    bind:open={confirmDeleteOpen}
    title={s.deleteDialogTitle}
    description={s.deleteDialogDescription}
    confirmLabel={s.delete}
    variant="destructive"
    onConfirm={remove}
  />
{/if}
