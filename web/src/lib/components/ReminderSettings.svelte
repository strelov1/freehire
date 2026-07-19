<script lang="ts">
  import { Bell, Check, Mail } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { api, ApiError } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { cn } from '$lib/utils';
  import ProviderIcon from './ProviderIcon.svelte';

  // The account-level saved-job reminder rule: turn reminders on, set the default
  // delay applied to new saves, and pick the delivery channels. Reminders are off
  // until the user enables them here. Scheduling itself happens per-save (with an
  // optional override); this block governs the default and the channels.

  let enabled = $state(false);
  let delayDays = $state(3);
  let channels = $state<string[]>([]);
  let telegramAvailable = $state(false);

  let status = $state<'loading' | 'ready' | 'error'>('loading');
  let saving = $state(false);
  let error = $state<string | null>(null);
  let savedOk = $state(false);

  const presets = [1, 3, 7, 14];

  $effect(() => {
    if (isAuthenticated()) void load();
  });

  async function load() {
    status = 'loading';
    try {
      const [settings, tg] = await Promise.all([api.getReminderSettings(), api.telegramStatus()]);
      enabled = settings.enabled;
      delayDays = settings.default_delay_days;
      channels = settings.channels;
      telegramAvailable = tg.enabled;
      status = 'ready';
    } catch {
      status = 'error';
    }
  }

  function dirty() {
    savedOk = false;
    error = null;
  }

  function toggleChannel(channel: string) {
    channels = channels.includes(channel) ? channels.filter((c) => c !== channel) : [...channels, channel];
    dirty();
  }

  async function save() {
    if (saving) return;
    saving = true;
    error = null;
    savedOk = false;
    try {
      const settings = await api.updateReminderSettings({
        enabled,
        default_delay_days: delayDays,
        channels,
      });
      enabled = settings.enabled;
      delayDays = settings.default_delay_days;
      channels = settings.channels;
      savedOk = true;
    } catch (e) {
      error = e instanceof ApiError ? e.message : 'Could not save your reminder settings.';
    } finally {
      saving = false;
    }
  }

  const chipClass = (on: boolean) =>
    cn(
      'inline-flex items-center gap-1.5 rounded-full border px-3 py-1.5 text-xs font-semibold transition-colors disabled:opacity-50',
      on
        ? 'border-transparent bg-brand-muted text-brand-strong'
        : 'border-border bg-background text-muted-foreground hover:border-muted-foreground/40',
    );
</script>

<section class="rounded-lg border border-border bg-card p-4">
  <div class="flex items-start justify-between gap-3">
    <div class="flex flex-col gap-0.5">
      <h2 class="flex items-center gap-1.5 text-sm font-semibold">
        <Bell class="size-4" aria-hidden="true" /> Reminders
      </h2>
      <p class="text-xs text-muted-foreground">Nudge me to come back to a saved job before it goes stale.</p>
    </div>
    <label class="inline-flex cursor-pointer items-center gap-2 text-xs font-medium">
      <input type="checkbox" bind:checked={enabled} onchange={dirty} disabled={status !== 'ready'} class="size-4 accent-brand" />
      {enabled ? 'On' : 'Off'}
    </label>
  </div>

  {#if status === 'error'}
    <p class="mt-3 text-xs text-destructive">Couldn't load your reminder settings.</p>
  {:else if enabled && status === 'ready'}
    <div class="mt-4 flex flex-col gap-4">
      <div class="flex flex-col gap-1.5">
        <span class="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">Remind me after</span>
        <div class="flex flex-wrap items-center gap-2">
          {#each presets as d (d)}
            <button
              type="button"
              onclick={() => {
                delayDays = d;
                dirty();
              }}
              class={chipClass(delayDays === d)}
            >
              {d === 1 ? '1 day' : `${d} days`}
            </button>
          {/each}
        </div>
      </div>

      <div class="flex flex-col gap-1.5">
        <span class="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">Deliver over</span>
        <div class="flex flex-wrap items-center gap-2">
          {#if telegramAvailable}
            <button type="button" onclick={() => toggleChannel('telegram')} aria-pressed={channels.includes('telegram')} class={chipClass(channels.includes('telegram'))}>
              {#if channels.includes('telegram')}
                <Check class="size-3.5" aria-hidden="true" />
              {:else}
                <ProviderIcon provider="telegram" class="size-3.5" />
              {/if}
              Telegram
            </button>
          {/if}
          <button type="button" onclick={() => toggleChannel('email')} aria-pressed={channels.includes('email')} class={chipClass(channels.includes('email'))}>
            {#if channels.includes('email')}
              <Check class="size-3.5" aria-hidden="true" />
            {:else}
              <Mail class="size-3.5" aria-hidden="true" />
            {/if}
            Email
          </button>
        </div>
        {#if channels.includes('telegram')}
          <p class="text-xs text-muted-foreground">Telegram reminders need the bot connected on your <a class="underline underline-offset-2" href={resolve('/my/searches')}>notifications</a> page.</p>
        {/if}
      </div>
    </div>
  {/if}

  {#if status === 'ready'}
    <div class="mt-4 flex items-center gap-3">
      <button
        type="button"
        onclick={save}
        disabled={saving || (enabled && channels.length === 0)}
        class="rounded-md bg-brand px-3 py-1.5 text-xs font-semibold text-brand-foreground transition-colors hover:opacity-90 disabled:opacity-50"
      >
        {saving ? 'Saving…' : 'Save'}
      </button>
      {#if enabled && channels.length === 0}
        <span class="text-xs text-muted-foreground">Pick at least one channel.</span>
      {:else if savedOk}
        <span class="flex items-center gap-1 text-xs text-muted-foreground"><Check class="size-3.5" aria-hidden="true" /> Saved</span>
      {:else if error}
        <span class="text-xs text-destructive">{error}</span>
      {/if}
    </div>
  {/if}
</section>
