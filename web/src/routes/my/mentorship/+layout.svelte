<script lang="ts">
  import type { Snippet } from 'svelte';
  import { browserTimezone } from '#lib/mentorship.js';

  // The mentor's panes (Booked with you, Schedule, Mentor profile) used to be a tab strip
  // here, beside the member's own booked sessions — two roles on one page. They are items
  // of the Mentor workspace's sidebar now (accountNav.ts), so this layout carries only the
  // section heading the two share.
  let { children }: { children: Snippet } = $props();

  // For the subtitle only. Each pane resolves the zone itself — it is a pure call, and
  // threading it down through context would cost more indirection than the repeat.
  const timezone = browserTimezone();
</script>

<!-- The account shell (my/+layout) owns the container, auth gate and noindex. -->
<div class="flex flex-col gap-6">
  <div class="flex flex-col gap-1 max-lg:sr-only">
    <h1 class="text-2xl font-semibold tracking-tight">Mentorship</h1>
    <p class="text-muted-foreground text-sm">
      Half-hour conversations with people who work where you want to. Times shown in {timezone}.
    </p>
  </div>

  {@render children()}
</div>
