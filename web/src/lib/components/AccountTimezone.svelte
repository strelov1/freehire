<script lang="ts">
  import { Check } from '@lucide/svelte';
  import { currentUser, updateTimezone } from '#lib/auth.svelte.js';
  import { ApiError } from '#lib/api.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { messages } from './AccountTimezone.messages';

  const s = $derived(t(messages, locale()));

  // The account's IANA timezone: read from the resolved session (no extra fetch —
  // it rides GET /me already). Used to interpret a daily search-alert digest time
  // and a quiet-hours window in the account's own local time
  // (internal/deliverywindow); unset reads as UTC there. A password registration
  // already captures the browser's detected zone (api.ts's register()), so this
  // mainly matters for a pre-existing account: if none is stored yet, the field
  // pre-fills with — and silently saves — the browser's detection the first time
  // this section renders, so a user never has to deliberately set it.
  // `Intl.supportedValuesOf('timeZone')` omits the bare "UTC" identifier (it only
  // lists Area/Location names) even though it's a real IANA zone Go's
  // time.LoadLocation accepts — prepend it so a stored/detected "UTC" has a
  // matching <option>, since a <select> silently shows nothing selected
  // otherwise. Newer engines (Chrome 118+, Safari 17+) DO list "UTC", so the
  // Set collapses the prepend into their entry rather than keying two
  // identical <option>s off it.
  const ZONES: string[] = [
    ...new Set([
      'UTC',
      ...(typeof Intl.supportedValuesOf === 'function' ? Intl.supportedValuesOf('timeZone') : []),
    ]),
  ];

  let detected = '';
  try {
    detected = Intl.DateTimeFormat().resolvedOptions().timeZone;
  } catch {
    detected = '';
  }

  let value = $state(currentUser()?.timezone ?? detected);
  let saveState = $state<'idle' | 'saving' | 'saved' | 'error'>('idle');
  // The server's own message when it gave one; `null` means the generic fallback,
  // resolved from `s` at render time so it follows a later locale change instead
  // of freezing in whatever locale was resolved when the save failed.
  let saveErrorMessage = $state<string | null>(null);
  let savedTimer: ReturnType<typeof setTimeout> | undefined;

  // Re-seed from the session on a real identity change (sign-in/out, or this
  // component's own save round-tripping through invalidateAll) — keyed on email
  // rather than running every render, so a value mid-pick is never clobbered by
  // an unrelated session refresh. Auto-persists the detected zone once for an
  // account that has never had one stored.
  let seededFor: string | null = null;
  $effect(() => {
    const user = currentUser();
    if (!user || user.email === seededFor) return;
    seededFor = user.email;
    if (user.timezone) {
      value = user.timezone;
    } else if (detected) {
      value = detected;
      void save();
    }
  });

  async function save() {
    if (!value) return;
    saveState = 'saving';
    saveErrorMessage = null;
    try {
      await updateTimezone(value);
      saveState = 'saved';
      clearTimeout(savedTimer);
      savedTimer = setTimeout(() => {
        if (saveState === 'saved') saveState = 'idle';
      }, 1500);
    } catch (e) {
      saveState = 'error';
      saveErrorMessage = e instanceof ApiError ? e.message : null;
    }
  }
</script>

<!-- One account setting's row: the heading, its save state, and the control. The card
     around it belongs to the caller, which groups this with the other account settings
     rather than boxing each one on its own. -->
<div class="flex flex-col gap-3">
  <div class="flex items-center gap-3">
    <div class="min-w-0 flex-1">
      <h2 class="text-sm font-semibold leading-tight">{s.heading}</h2>
      <p class="text-xs text-muted-foreground">
        {s.description}
      </p>
    </div>

    {#if saveState === 'saving'}
      <span class="text-xs text-muted-foreground">{s.saving}</span>
    {:else if saveState === 'saved'}
      <span class="flex items-center gap-1 text-xs text-brand-strong"><Check class="size-3.5" aria-hidden="true" /> {s.saved}</span>
    {:else if saveState === 'error'}
      <span class="text-xs text-destructive">{saveErrorMessage ?? s.saveFailed}</span>
    {/if}
  </div>

  <select
    bind:value
    onchange={save}
    class="w-full max-w-sm rounded-md border border-border bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
  >
    {#if !value}
      <option value="" disabled selected>{s.selectPlaceholder}</option>
    {/if}
    {#each ZONES as zone (zone)}
      <option value={zone}>{zone}</option>
    {/each}
  </select>
</div>
