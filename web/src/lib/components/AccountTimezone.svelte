<script lang="ts">
  import { Check, Clock } from '@lucide/svelte';
  import { currentUser, updateTimezone } from '$lib/auth.svelte';
  import { ApiError } from '$lib/api';

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
  let saveError = $state<string | null>(null);
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
    saveError = null;
    try {
      await updateTimezone(value);
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
</script>

<!-- One account setting's row: the heading, its save state, and the control. The card
     around it belongs to the caller, which groups this with the other account settings
     rather than boxing each one on its own. -->
<div class="flex flex-col gap-3">
  <div class="flex items-center gap-3">
    <div class="grid size-9 shrink-0 place-items-center rounded-lg bg-brand-muted text-brand-strong">
      <Clock class="size-4.5" aria-hidden="true" />
    </div>
    <div class="min-w-0 flex-1">
      <h2 class="text-sm font-semibold leading-tight">Timezone</h2>
      <p class="text-xs text-muted-foreground">
        Used to schedule a daily search-alert digest and quiet hours at your own local time.
      </p>
    </div>

    {#if saveState === 'saving'}
      <span class="text-xs text-muted-foreground">Saving…</span>
    {:else if saveState === 'saved'}
      <span class="flex items-center gap-1 text-xs text-brand-strong"><Check class="size-3.5" aria-hidden="true" /> Saved</span>
    {:else if saveState === 'error'}
      <span class="text-xs text-destructive">{saveError}</span>
    {/if}
  </div>

  <select
    bind:value
    onchange={save}
    class="w-full max-w-sm rounded-md border border-border bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
  >
    {#if !value}
      <option value="" disabled selected>Select a timezone</option>
    {/if}
    {#each ZONES as zone (zone)}
      <option value={zone}>{zone}</option>
    {/each}
  </select>
</div>
