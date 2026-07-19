<script lang="ts">
  import { Bell, Check, Mail } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { api, ApiError } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { cn } from '$lib/utils';
  import ProviderIcon from './ProviderIcon.svelte';

  // The account-level saved-job reminder rule: turn reminders on, set the default
  // delay applied to new saves, and pick the delivery channels. Reminders are off
  // until the user enables them here. Changes autosave — there is no Save button.
  // The UI keeps the rule valid so an invalid payload never reaches the server:
  // enabling defaults to the email channel, and the last channel can't be removed
  // while reminders are on (an enabled rule requires at least one channel).

  let enabled = $state(false);
  let delayDays = $state(3);
  let channels = $state<string[]>([]);
  let telegramAvailable = $state(false);

  let status = $state<'loading' | 'ready' | 'error'>('loading');
  let saveState = $state<'idle' | 'saving' | 'saved' | 'error'>('idle');
  let saveError = $state<string | null>(null);

  const presets = [1, 3, 7, 14];

  let saveTimer: ReturnType<typeof setTimeout> | undefined;
  let savedTimer: ReturnType<typeof setTimeout> | undefined;

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

  // Debounced autosave: batches rapid clicks into one write, then shows a brief
  // "Saved". The UI only ever holds valid state, so the write can't 400.
  function persist() {
    saveState = 'saving';
    saveError = null;
    clearTimeout(saveTimer);
    saveTimer = setTimeout(doSave, 350);
  }

  async function doSave() {
    try {
      await api.updateReminderSettings({ enabled, default_delay_days: delayDays, channels });
      saveState = 'saved';
      clearTimeout(savedTimer);
      savedTimer = setTimeout(() => {
        if (saveState === 'saved') saveState = 'idle';
      }, 1500);
    } catch (e) {
      saveState = 'error';
      saveError = e instanceof ApiError ? e.message : 'Could not save.';
    }
  }

  function toggleEnabled() {
    if (status !== 'ready') return;
    enabled = !enabled;
    // Enabling needs a channel; default to email (always deliverable to the account).
    if (enabled && channels.length === 0) channels = ['email'];
    persist();
  }

  function toggleChannel(channel: string) {
    const has = channels.includes(channel);
    // Keep at least one channel while reminders are on — the invariant the server enforces.
    if (has && enabled && channels.length === 1) return;
    channels = has ? channels.filter((c) => c !== channel) : [...channels, channel];
    persist();
  }

  function setDelay(d: number) {
    delayDays = d;
    persist();
  }

  const pill = (on: boolean) =>
    cn(
      'inline-flex items-center gap-1.5 rounded-full border px-3 py-1.5 text-xs font-semibold transition-colors',
      on
        ? 'border-transparent bg-brand-muted text-brand-strong'
        : 'border-border bg-background text-muted-foreground hover:border-muted-foreground/40 hover:text-foreground',
    );
</script>

<section class="rounded-xl border border-border bg-card p-4">
  <div class="flex items-center gap-3">
    <div class="grid size-9 shrink-0 place-items-center rounded-lg bg-brand-muted text-brand-strong">
      <Bell class="size-4.5" aria-hidden="true" />
    </div>
    <div class="min-w-0 flex-1">
      <h2 class="text-sm font-semibold leading-tight">Reminders</h2>
      <p class="text-xs text-muted-foreground">Nudge me to come back to a saved job before it goes stale.</p>
    </div>

    <!-- Autosave status, then the on/off toggle. -->
    {#if saveState === 'saving'}
      <span class="text-xs text-muted-foreground">Saving…</span>
    {:else if saveState === 'saved'}
      <span class="flex items-center gap-1 text-xs text-brand-strong"><Check class="size-3.5" aria-hidden="true" /> Saved</span>
    {:else if saveState === 'error'}
      <span class="text-xs text-destructive">{saveError}</span>
    {/if}

    <button
      type="button"
      role="switch"
      aria-checked={enabled}
      aria-label="Enable reminders"
      onclick={toggleEnabled}
      disabled={status !== 'ready'}
      class={cn(
        'relative inline-flex h-6 w-11 shrink-0 items-center rounded-full transition-colors disabled:opacity-50',
        'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-ring focus-visible:ring-offset-2 focus-visible:ring-offset-card',
        enabled ? 'bg-brand' : 'bg-muted',
      )}
    >
      <span
        class={cn(
          'inline-block size-5 rounded-full bg-white shadow-sm transition-transform',
          enabled ? 'translate-x-[22px]' : 'translate-x-0.5',
        )}
      ></span>
    </button>
  </div>

  {#if status === 'error'}
    <p class="mt-4 text-xs text-destructive">Couldn't load your reminder settings.</p>
  {:else if enabled && status === 'ready'}
    <div class="mt-4 flex flex-col gap-4 border-t border-border pt-4">
      <div class="flex flex-col gap-2">
        <span class="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">Remind me after</span>
        <div class="flex flex-wrap items-center gap-2">
          {#each presets as d (d)}
            <button type="button" onclick={() => setDelay(d)} class={pill(delayDays === d)}>
              {d === 1 ? '1 day' : `${d} days`}
            </button>
          {/each}
        </div>
      </div>

      <div class="flex flex-col gap-2">
        <span class="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">Deliver over</span>
        <div class="flex flex-wrap items-center gap-2">
          {#if telegramAvailable}
            <button type="button" onclick={() => toggleChannel('telegram')} aria-pressed={channels.includes('telegram')} class={pill(channels.includes('telegram'))}>
              {#if channels.includes('telegram')}
                <Check class="size-3.5" aria-hidden="true" />
              {:else}
                <ProviderIcon provider="telegram" class="size-3.5" />
              {/if}
              Telegram
            </button>
          {/if}
          <button type="button" onclick={() => toggleChannel('email')} aria-pressed={channels.includes('email')} class={pill(channels.includes('email'))}>
            {#if channels.includes('email')}
              <Check class="size-3.5" aria-hidden="true" />
            {:else}
              <Mail class="size-3.5" aria-hidden="true" />
            {/if}
            Email
          </button>
        </div>
        {#if channels.includes('telegram')}
          <p class="text-xs text-muted-foreground">Telegram reminders need the bot connected on your <a class="font-medium text-foreground underline underline-offset-2 hover:opacity-80" href={resolve('/my/searches')}>notifications</a> page.</p>
        {/if}
      </div>
    </div>
  {/if}
</section>
