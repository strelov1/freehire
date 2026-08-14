<script lang="ts">
  import { Bell, Check, Mail, Moon, Smartphone } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { api, ApiError } from '$lib/api';
  import { currentUser, isAuthenticated } from '$lib/auth.svelte';
  import { cn } from '$lib/ui';
  import { ProviderIcon } from '$lib/ui';

  // The single account-level notification rule: turn notifications on and pick
  // the delivery channels. It gates four things at once — the saved-job apply
  // reminder, the follow-up nudge (an application has gone quiet), the
  // interview-prep nudge (a stage moved to interview), and the job-closed nudge
  // (a tracked listing closed) — there is no separate control for any of them,
  // and no per-job override. Changes autosave — there is no Save button. The UI
  // keeps the rule valid so an invalid payload never reaches the server: enabling
  // defaults to the email channel, and the last channel can't be removed while
  // notifications are on (an enabled rule requires at least one channel).
  //
  // Two more controls ride the same settings row but govern different things:
  // digest frequency (instant/daily) affects ONLY saved-search alerts
  // (internal/notify), not the reminder/nudge kinds above; quiet hours defers
  // ALL of them (plus instant-frequency search alerts) except a `daily` digest,
  // which fires at its own chosen time regardless.

  let enabled = $state(false);
  let channels = $state<string[]>([]);
  let telegramAvailable = $state(false);

  let digestFrequency = $state('instant');
  let digestTime = $state('09:00');
  let quietHoursOn = $state(false);
  let quietHoursStart = $state('22:00');
  let quietHoursEnd = $state('08:00');

  let status = $state<'loading' | 'ready' | 'error'>('loading');
  let saveState = $state<'idle' | 'saving' | 'saved' | 'error'>('idle');
  let saveError = $state<string | null>(null);

  let saveTimer: ReturnType<typeof setTimeout> | undefined;
  let savedTimer: ReturnType<typeof setTimeout> | undefined;

  const hasTimezone = $derived(Boolean(currentUser()?.timezone));

  $effect(() => {
    if (isAuthenticated()) void load();
  });

  async function load() {
    status = 'loading';
    try {
      const [settings, tg] = await Promise.all([api.getNotificationSettings(), api.telegramStatus()]);
      enabled = settings.enabled;
      channels = settings.channels;
      telegramAvailable = tg.enabled;
      digestFrequency = settings.digest_frequency || 'instant';
      digestTime = settings.digest_time ?? '09:00';
      quietHoursOn = Boolean(settings.quiet_hours_start && settings.quiet_hours_end);
      quietHoursStart = settings.quiet_hours_start ?? '22:00';
      quietHoursEnd = settings.quiet_hours_end ?? '08:00';
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
      await api.updateNotificationSettings({
        enabled,
        channels,
        digest_frequency: digestFrequency,
        digest_time: digestFrequency === 'daily' ? digestTime : null,
        quiet_hours_start: quietHoursOn ? quietHoursStart : null,
        quiet_hours_end: quietHoursOn ? quietHoursEnd : null,
      });
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
    // Keep at least one channel while notifications are on — the invariant the server enforces.
    if (has && enabled && channels.length === 1) return;
    channels = has ? channels.filter((c) => c !== channel) : [...channels, channel];
    persist();
  }

  function setDigestFrequency(freq: string) {
    digestFrequency = freq;
    persist();
  }

  function toggleQuietHours() {
    quietHoursOn = !quietHoursOn;
    persist();
  }

  const pill = (on: boolean) =>
    cn(
      'inline-flex items-center gap-1.5 rounded-full border px-3 py-1.5 text-xs font-semibold transition-colors',
      on
        ? 'border-transparent bg-brand-muted text-brand-strong'
        : 'border-border bg-background text-muted-foreground hover:border-muted-foreground/40 hover:text-foreground',
    );

  // Shared by every section label below, so the arbitrary 11px size is written once
  // rather than repeated per section (design-system/scripts/check-token-coverage.mjs
  // counts occurrences, not unique values).
  const sectionLabel = 'text-[11px] font-semibold uppercase tracking-wider text-muted-foreground';
</script>

<section class="rounded-xl border border-border bg-card p-4">
  <div class="flex items-center gap-3">
    <div class="grid size-9 shrink-0 place-items-center rounded-lg bg-brand-muted text-brand-strong">
      <Bell class="size-4.5" aria-hidden="true" />
    </div>
    <div class="min-w-0 flex-1">
      <h2 class="text-sm font-semibold leading-tight">Notifications</h2>
      <p class="text-xs text-muted-foreground">
        Come back to a saved job before it goes stale, follow up when an application goes quiet, prepare before an interview, and hear about it if a listing you're tracking closes.
      </p>
    </div>

    <!-- Autosave status, then the on/off toggle. -->
    {#if saveState === 'saving'}
      <span class="text-xs text-muted-foreground">Saving…</span>
    {:else if saveState === 'saved'}
      <span class="flex items-center gap-1 text-xs text-brand-strong"><Check class="size-3.5" aria-hidden="true" /> Saved</span>
    {:else if saveState === 'error'}
      <span class="text-xs text-destructive">{saveError}</span>
    {/if}

    {@render toggleSwitch(enabled, 'Enable notifications', toggleEnabled, status !== 'ready')}
  </div>

  {#if status === 'error'}
    <p class="mt-4 text-xs text-destructive">Couldn't load your notification settings.</p>
  {:else if enabled && status === 'ready'}
    <div class="mt-4 flex flex-col gap-4 border-t border-border pt-4">
      <div class="flex flex-col gap-2">
        <span class={sectionLabel}>Deliver over</span>
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
          <button type="button" onclick={() => toggleChannel('push')} aria-pressed={channels.includes('push')} class={pill(channels.includes('push'))}>
            {#if channels.includes('push')}
              <Check class="size-3.5" aria-hidden="true" />
            {:else}
              <Smartphone class="size-3.5" aria-hidden="true" />
            {/if}
            Push
          </button>
        </div>
        {#if channels.includes('telegram')}
          <p class="text-xs text-muted-foreground">Telegram notifications need the bot connected on the <a class="font-medium text-foreground underline underline-offset-2 hover:opacity-80" href={resolve('/my/notifications/searches')}>Search alerts</a> tab.</p>
        {/if}
      </div>

      <div class="flex flex-col gap-2 border-t border-border pt-4">
        <span class={sectionLabel}>Timing</span>
        <dl class="flex flex-col gap-1.5 text-xs text-muted-foreground">
          <div class="flex gap-1.5">
            <dt class="shrink-0 font-medium text-foreground">Apply reminder —</dt>
            <dd>3 days after you save a job you haven't applied to yet.</dd>
          </div>
          <div class="flex gap-1.5">
            <dt class="shrink-0 font-medium text-foreground">Follow-up —</dt>
            <dd>once an application goes quiet: 21 days after applying, 18 in screening, 15 after a reply, 12 once interviewing, 5 with an offer out.</dd>
          </div>
          <div class="flex gap-1.5">
            <dt class="shrink-0 font-medium text-foreground">Interview prep —</dt>
            <dd>right when you move a card to Interview.</dd>
          </div>
          <div class="flex gap-1.5">
            <dt class="shrink-0 font-medium text-foreground">Listing closed —</dt>
            <dd>right when a job you're still actively tracking closes — the card also moves to Expired on your board.</dd>
          </div>
        </dl>
      </div>
    </div>
  {/if}
</section>

<!-- Delivery timing: digest frequency governs only saved-search alerts
     (internal/notify), independent of the Notifications toggle above — a
     search-alert subscriber with reminders/nudges off still picks a frequency.
     Quiet hours defers everything except a `daily` digest, which is exempt by
     design (a chosen time is itself the preference). Both stay visible/editable
     regardless of the toggle above, and use the same debounced-autosave PUT. -->
{#if status === 'ready'}
  <section class="mt-4 rounded-xl border border-border bg-card p-4">
    <div class="flex flex-col gap-2">
      <span class={sectionLabel}>Search alert frequency</span>
      <div class="flex flex-wrap items-center gap-2">
        <button
          type="button"
          onclick={() => setDigestFrequency('instant')}
          aria-pressed={digestFrequency === 'instant'}
          class={pill(digestFrequency === 'instant')}
        >
          {#if digestFrequency === 'instant'}<Check class="size-3.5" aria-hidden="true" />{/if}
          Instant
        </button>
        <button
          type="button"
          onclick={() => setDigestFrequency('daily')}
          aria-pressed={digestFrequency === 'daily'}
          class={pill(digestFrequency === 'daily')}
        >
          {#if digestFrequency === 'daily'}<Check class="size-3.5" aria-hidden="true" />{/if}
          Daily digest
        </button>
        {#if digestFrequency === 'daily'}
          <input
            type="time"
            bind:value={digestTime}
            onchange={persist}
            aria-label="Daily digest time"
            class="rounded-md border border-border bg-background px-2 py-1.5 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
        {/if}
      </div>
      <p class="text-xs text-muted-foreground">
        Applies only to saved-search alerts — reminders and application nudges above always deliver as they happen.
      </p>
      {#if digestFrequency === 'daily' && !hasTimezone}
        <p class="text-xs text-muted-foreground">
          Set your <a class="font-medium text-foreground underline underline-offset-2 hover:opacity-80" href={resolve('/my/profile')}>timezone</a> so the digest time means your own local time — until then it's read as UTC.
        </p>
      {/if}
    </div>

    <div class="mt-4 flex flex-col gap-2 border-t border-border pt-4">
      <div class="flex items-center gap-3">
        <div class="grid size-9 shrink-0 place-items-center rounded-lg bg-brand-muted text-brand-strong">
          <Moon class="size-4.5" aria-hidden="true" />
        </div>
        <div class="min-w-0 flex-1">
          <span class={sectionLabel}>Quiet hours</span>
          <p class="text-xs text-muted-foreground">
            Delay reminders, nudges, and instant search alerts until this window ends. A daily digest still arrives at its own time.
          </p>
        </div>
        {@render toggleSwitch(quietHoursOn, 'Enable quiet hours', toggleQuietHours, false)}
      </div>
      {#if quietHoursOn}
        <div class="flex items-center gap-2 pl-12">
          <input
            type="time"
            bind:value={quietHoursStart}
            onchange={persist}
            aria-label="Quiet hours start"
            class="rounded-md border border-border bg-background px-2 py-1.5 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
          <span class="text-xs text-muted-foreground">to</span>
          <input
            type="time"
            bind:value={quietHoursEnd}
            onchange={persist}
            aria-label="Quiet hours end"
            class="rounded-md border border-border bg-background px-2 py-1.5 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
        </div>
        {#if !hasTimezone}
          <p class="pl-12 text-xs text-muted-foreground">
            Set your <a class="font-medium text-foreground underline underline-offset-2 hover:opacity-80" href={resolve('/my/profile')}>timezone</a> so this means your own local time — until then it's read as UTC.
          </p>
        {/if}
      {/if}
    </div>
  </section>
{/if}

{#snippet toggleSwitch(on: boolean, label: string, onToggle: () => void, disabled: boolean)}
  <button
    type="button"
    role="switch"
    aria-checked={on}
    aria-label={label}
    onclick={onToggle}
    {disabled}
    class={cn(
      'relative inline-flex h-6 w-11 shrink-0 items-center rounded-full transition-colors disabled:opacity-50',
      'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-ring focus-visible:ring-offset-2 focus-visible:ring-offset-card',
      on ? 'bg-brand' : 'bg-muted',
    )}
  >
    <span
      class={cn(
        'inline-block size-5 rounded-full bg-white shadow-sm transition-transform',
        on ? 'translate-x-[22px]' : 'translate-x-0.5',
      )}
    ></span>
  </button>
{/snippet}
