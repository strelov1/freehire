<script lang="ts">
  import { resolve } from '$app/paths';
  import { onMount } from 'svelte';
  import { initTheme } from '$lib/theme.svelte';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { resetViewedJobs } from '$lib/viewedJobs.svelte';
  import { savedSearches } from '$lib/savedSearches.svelte';
  import { searchProfiles } from '$lib/searchProfiles.svelte';
  import { notifications } from '$lib/notifications.svelte';
  import TopBar from '$lib/components/TopBar.svelte';
  import ProviderIcon from '$lib/components/ProviderIcon.svelte';
  import '../app.css';

  let { children } = $props();

  // Apply the persisted theme and start tracking the OS preference once mounted.
  // A no-FOUC inline script in app.html already set the class before paint.
  onMount(() => initTheme());

  // Drop every per-user store the moment the session ends. logout() re-resolves via
  // invalidateAll() — a soft client navigation, so these module-singleton stores
  // survive it and would otherwise show the previous user's data to whoever signs in
  // next on the same tab. Centralized in the always-mounted root layout so it fires
  // regardless of route (the per-component load effects only cover their own store
  // while mounted; viewedJobs, used on /jobs, had no reset at all). Idempotent — a
  // reset of already-empty stores while signed out is a no-op.
  $effect(() => {
    if (!isAuthenticated()) {
      resetViewedJobs();
      savedSearches.reset();
      searchProfiles.reset();
      notifications.reset();
    }
  });
</script>

<!-- Column layout with min-h-svh (small-viewport height, so the mobile address
     bar never forces extra scroll) keeps the footer pinned to the bottom on
     sparse pages while letting main grow. Each route owns its own width/padding. -->
<div class="flex min-h-svh flex-col">
  <TopBar />

  <main class="flex-1">
    {@render children()}
  </main>

  <footer class="border-t border-border">
    <div
      class="mx-auto flex w-full max-w-6xl items-center justify-between gap-3 px-4 py-3 text-xs text-muted-foreground"
    >
      <p>Free, open-source IT job aggregator.</p>
      <div class="flex shrink-0 items-center gap-4">
        <a
          href={resolve('/cli')}
          class="shrink-0 font-medium text-foreground transition-colors hover:text-muted-foreground"
        >
          CLI
        </a>
        <a
          href={resolve('/docs/api')}
          class="shrink-0 font-medium text-foreground transition-colors hover:text-muted-foreground"
        >
          API
        </a>
        <a
          href="https://t.me/freehiredev"
          target="_blank"
          rel="noopener noreferrer"
          class="inline-flex shrink-0 items-center gap-1.5 font-medium text-foreground transition-colors hover:text-muted-foreground"
        >
          <ProviderIcon provider="telegram" /> Ask a question
        </a>
        <a
          href="https://github.com/strelov1/freehire"
          target="_blank"
          rel="noopener noreferrer"
          class="inline-flex shrink-0 items-center gap-1.5 font-medium text-foreground transition-colors hover:text-muted-foreground"
        >
          <ProviderIcon provider="github" /> GitHub
        </a>
      </div>
    </div>
  </footer>
</div>
