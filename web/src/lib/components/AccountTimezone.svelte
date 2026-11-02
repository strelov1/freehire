<script lang="ts">
  import { currentUser, updateTimezone } from '#lib/auth.svelte.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { SaveStatus } from '#lib/saveStatus.svelte.js';
  import SaveStatusLine from './SaveStatus.svelte';
  import { messages } from './AccountTimezone.messages';

  const s = $derived(t(messages, locale()));
  // Names the <select> after the visible heading — it has no <label> of its own.
  const headingId = $props.id();

  // The root layout persists the detected zone for accounts without one. This
  // field shows only the stored value so a failed adoption can be retried by picking
  // any zone, including the browser's. Explicit picks save here.
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

  let value = $state(currentUser()?.timezone ?? '');
  const status = new SaveStatus();
  let pendingSave = $state<ReturnType<typeof setTimeout>>();

  // Follow adoption and other session refreshes unless a local pick is pending or
  // failed. A real identity change discards that account's edit state.
  let seededFor: string | null = null;
  $effect(() => {
    const user = currentUser();
    if ((user?.email ?? null) !== seededFor) {
      seededFor = user?.email ?? null;
      clearTimeout(pendingSave);
      pendingSave = undefined;
      value = user?.timezone ?? '';
    } else if (pendingSave === undefined && !status.saving && status.state !== 'error') {
      value = user?.timezone ?? '';
    }
  });

  // A closed <select> on Windows changes its value on every arrow key, firing `change` each
  // time — so a save per change wrote every zone the user stepped past. The save waits for
  // the value to settle, and leaving the control commits whatever is pending at once.
  const SAVE_DEBOUNCE_MS = 600;

  function scheduleSave() {
    clearTimeout(pendingSave);
    pendingSave = setTimeout(commitSave, SAVE_DEBOUNCE_MS);
  }

  function commitSave() {
    if (pendingSave === undefined) return;
    clearTimeout(pendingSave);
    pendingSave = undefined;
    void save();
  }

  // A pick made just before navigating away still lands — quietly: the row that would
  // show its outcome is gone, and a "Saved" spoken over the next page belongs to nothing
  // on it.
  let destroyed = false;
  $effect(() => () => {
    destroyed = true;
    commitSave();
  });

  async function save() {
    const zone = value;
    if (!zone) return;
    await status.run(() => updateTimezone(zone), {}, destroyed);
  }
</script>

<!-- One account setting's row: the heading, its save state, and the control. The card
     around it belongs to the caller, which groups this with the other account settings
     rather than boxing each one on its own. -->
<div class="flex flex-col gap-3">
  <div class="flex items-center gap-3">
    <div class="min-w-0 flex-1">
      <h2 id={headingId} class="text-sm font-semibold leading-tight">{s.heading}</h2>
      <p class="text-xs text-muted-foreground">
        {s.description}
      </p>
    </div>

    <SaveStatusLine {status} />
  </div>

  <select
    bind:value
    onchange={scheduleSave}
    onblur={commitSave}
    aria-labelledby={headingId}
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
