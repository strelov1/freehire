<script lang="ts">
  import { onMount } from 'svelte';
  import { Bell, Bookmark, ChevronRight } from '@lucide/svelte';
  import { ApiError } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { openAuthDialog } from '$lib/auth-dialog.svelte';
  import { savedSearches } from '$lib/savedSearches.svelte';
  import { notifications } from '$lib/notifications.svelte';
  import { ensureSaved, matchedSavedSearch, setPendingAlert } from '$lib/saveSearchAlert';
  import { Button } from '$lib/ui';
  import AlertChannels from './AlertChannels.svelte';

  // The "save this search, then get its new jobs" control, driven by a query string so
  // the same behavior serves the sidebar, the onboarding banner, and the modal's "My
  // filters" tab. Save is the primary, standalone action (works without any alert); once
  // saved, the per-channel alert toggles (AlertChannels) take over. The save half's
  // orchestration lives in saveSearchAlert.ts.
  //
  // `variant`: `quick` = the compact onboarding offer; `full` = the prominent placement
  // (modal / sidebar). `alerts`: `inline` renders the channel toggles in place once
  // saved; `manage` (the compact sidebar) links out to /my/searches instead of managing
  // channels inline.
  let {
    query,
    variant = 'full',
    alerts = 'inline',
    autostart = false,
  }: {
    query: string;
    variant?: 'quick' | 'full';
    alerts?: 'inline' | 'manage';
    /** Run the save on mount — used to resume a pending save after sign-in. */
    autostart?: boolean;
  } = $props();

  let busy = $state(false);
  let error = $state<string | null>(null);

  // The primary Save button is prominent in the `full` placement (modal / sidebar) and
  // compact in the `quick` onboarding banner. Derived once so both save states share it.
  const saveBig = $derived(variant === 'full');
  const saveBtnClass = $derived(
    saveBig
      ? 'w-full justify-center gap-2 rounded-xl text-[0.95rem] font-semibold shadow-sm transition-shadow hover:shadow-md'
      : 'justify-center gap-2 rounded-lg font-semibold',
  );

  // Load saved searches + telegram status once the session is known; sign-out cache
  // reset is owned centrally by +layout.svelte, so it isn't duplicated here.
  $effect(() => {
    if (isAuthenticated()) {
      void savedSearches.ensureLoaded();
      void notifications.ensureLoaded();
    }
  });

  const matched = $derived(matchedSavedSearch(query, savedSearches.items));

  // Save the current filters (auth-gated). Standalone: no alert channel involved — once
  // it lands, `matched` flips and the channel toggles / manage link render.
  async function save() {
    if (busy) return;
    if (!isAuthenticated()) {
      setPendingAlert(query); // resume the save after sign-in (incl. an OAuth redirect)
      openAuthDialog();
      return;
    }
    busy = true;
    error = null;
    try {
      await ensureSaved(query, savedSearches);
    } catch (e) {
      error = e instanceof ApiError ? e.message : 'Could not save. Please try again.';
    } finally {
      busy = false;
    }
  }

  // Resume a pending save (after sign-in): run it exactly once. onMount (not a $effect)
  // so it can't self-invalidate through save()'s reactive reads and loop.
  onMount(() => {
    if (autostart) void save();
  });
</script>

<div class="flex flex-col gap-1.5">
  {#if !isAuthenticated()}
    <Button variant="primary" size={saveBig ? 'lg' : 'md'} onclick={save} class={saveBtnClass}>
      <Bookmark class={saveBig ? 'size-[1.15rem]' : 'size-4'} aria-hidden="true" />
      Save this search
    </Button>
  {:else if !matched}
    <Button variant="primary" size={saveBig ? 'lg' : 'md'} onclick={save} disabled={busy} class={saveBtnClass}>
      <Bookmark class={saveBig ? 'size-[1.15rem]' : 'size-4'} aria-hidden="true" />
      {busy ? 'Saving…' : 'Save filter'}
    </Button>
  {:else if alerts === 'manage'}
    <Button
      variant="secondary"
      size="lg"
      href="/my/searches"
      class="w-full justify-center gap-2 rounded-xl font-semibold"
    >
      <Bell class="size-4" aria-hidden="true" />
      Get new-job alerts
      <ChevronRight class="size-4 text-muted-foreground" aria-hidden="true" />
    </Button>
  {:else}
    <AlertChannels savedSearchId={matched.id} />
  {/if}

  {#if error}
    <p class="text-sm text-destructive">{error}</p>
  {/if}
</div>
