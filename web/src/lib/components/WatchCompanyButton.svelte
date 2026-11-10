<script lang="ts">
  import { Eye } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { announce } from '#lib/announce.svelte.js';
  import { ApiError, api } from '#lib/api.js';
  import { isAuthenticated } from '#lib/auth.svelte.js';
  import { promptSignIn } from '#lib/signin.js';
  import { notifications } from '#lib/notifications.svelte.js';
  import { savedSearches } from '#lib/savedSearches.svelte.js';
  import { matchedSavedSearch } from '#lib/saveSearchAlert.js';
  import { Button } from '#lib/ui/index.js';

  // "Watch" on a company page: put the company on the personal watchlist whose new
  // postings the watchlist alert delivers. The state is read in the browser after mount,
  // never in `load`, so the public page stays identical for every visitor and cacheable.
  let { slug, companyName }: { slug: string; companyName: string } = $props();

  let watched = $state(false);
  let busy = $state(false);
  let error = $state<string | null>(null);
  // Set by a successful Watch click, never on load: the hint answers "where do the alerts
  // come from?" at the moment of asking, not on every visit to a company already watched.
  let justWatched = $state(false);
  // A lookup that started before a successful click read the list as it was, so it must not
  // overwrite what the click just did.
  let toggled = false;

  // The watchlist alert is the saved search `watchlist=true` with a channel (WatchlistAlert).
  const alertOn = $derived.by(() => {
    const alert = matchedSavedSearch('watchlist=true', savedSearches.items);
    return !!alert && notifications.subscriptions.some((s) => s.saved_search_id === alert.id);
  });

  $effect(() => {
    const target = slug;
    watched = false;
    error = null;
    justWatched = false;
    toggled = false;
    if (!isAuthenticated()) return;
    let current = true;
    api
      .isWatched(target)
      .then((on) => {
        if (current && !toggled) watched = on;
      })
      .catch(() => {});
    return () => {
      current = false;
    };
  });

  async function toggle() {
    if (busy) return;
    if (!isAuthenticated()) {
      promptSignIn();
      return;
    }
    busy = true;
    error = null;
    // The page can move to another company while a request is out; a late answer about the
    // old one must not land on the new one's button.
    const target = slug;
    const name = companyName;
    try {
      if (watched) {
        await api.unwatchCompany(target);
        if (target !== slug) return;
        toggled = true;
        watched = false;
        justWatched = false;
        announce(`Stopped watching ${name}`);
      } else {
        await api.watchCompany(target);
        if (target !== slug) return;
        toggled = true;
        watched = true;
        // Loaded only now, so a company page costs nobody these two reads until they watch,
        // and awaited so the hint does not flash for someone whose alert is already on.
        await Promise.all([savedSearches.ensureLoaded(), notifications.ensureLoaded()]).catch(() => {});
        if (target !== slug) return;
        justWatched = true;
        announce(`Watching ${name}`);
      }
    } catch (e) {
      if (target !== slug) return;
      error = e instanceof ApiError ? e.message : 'Could not update the watchlist. Please try again.';
      announce(error, 'assertive');
    } finally {
      busy = false;
    }
  }
</script>

<!-- No box of its own: the button and the lines under it are items of the header's wrapping
     row, so the lines can take a full row instead of widening the button's column. -->
<div class="contents">
  <Button
    class="shrink-0"
    variant={watched ? 'secondary' : 'outline'}
    size="sm"
    onclick={toggle}
    disabled={busy}
    aria-pressed={watched}
    aria-label={watched ? 'Watching' : 'Watch'}
  >
    <Eye class="size-4" aria-hidden="true" />
    <span class="hidden sm:inline">{watched ? 'Watching' : 'Watch'}</span>
  </Button>
  {#if justWatched && !alertOn}
    <p class="basis-full text-right text-xs text-muted-foreground">
      Added to your watchlist.
      <a href={resolve('/my/watchlist')} class="font-medium text-foreground underline underline-offset-2">
        Turn on alerts
      </a>
      to hear about new jobs.
    </p>
  {/if}
  {#if error}
    <p class="basis-full text-right text-xs text-destructive">{error}</p>
  {/if}
</div>
