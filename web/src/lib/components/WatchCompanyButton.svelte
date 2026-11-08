<script lang="ts">
  import { Eye } from '@lucide/svelte';
  import { announce } from '#lib/announce.svelte.js';
  import { ApiError, api } from '#lib/api.js';
  import { isAuthenticated } from '#lib/auth.svelte.js';
  import { promptSignIn } from '#lib/signin.js';
  import { Button } from '#lib/ui/index.js';

  // "Watch" on a company page: put the company on the personal watchlist whose new
  // postings the watchlist alert delivers. The state is read in the browser after mount,
  // never in `load`, so the public page stays identical for every visitor and cacheable.
  let { slug, companyName }: { slug: string; companyName: string } = $props();

  let watched = $state(false);
  let busy = $state(false);
  let error = $state<string | null>(null);

  $effect(() => {
    const target = slug;
    watched = false;
    error = null;
    if (!isAuthenticated()) return;
    let current = true;
    api
      .isWatched(target)
      .then((on) => {
        if (current) watched = on;
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
    try {
      if (watched) {
        await api.unwatchCompany(slug);
        watched = false;
        announce(`Stopped watching ${companyName}`);
      } else {
        await api.watchCompany(slug);
        watched = true;
        announce(`Watching ${companyName}`);
      }
    } catch (e) {
      error = e instanceof ApiError ? e.message : 'Could not update the watchlist. Please try again.';
      announce(error, 'assertive');
    } finally {
      busy = false;
    }
  }
</script>

<div class="flex flex-col items-end gap-1">
  <Button
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
  {#if error}
    <p class="max-w-56 text-right text-xs text-destructive">{error}</p>
  {/if}
</div>
