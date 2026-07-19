<script lang="ts">
  import { api } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import type { AiCredits } from '$lib/types';

  // The caller's AI-credits balance: points spent on résumé match (1) and CV tailoring (3),
  // topped up by a monthly grant and by accepted board contributions. Read-only.
  let credits = $state<AiCredits | null>(null);

  $effect(() => {
    if (!isAuthenticated()) return;
    api
      .myCredits()
      .then((c) => (credits = c))
      .catch(() => {}); // best-effort: the widget just stays hidden on error
  });

  const fmtDate = (iso: string) =>
    new Date(iso).toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
</script>

{#if credits}
  <div
    class="flex items-center justify-between gap-3 rounded-lg border border-border bg-secondary/40 px-4 py-2.5 text-sm"
  >
    <span class="text-muted-foreground">
      <strong class="font-semibold text-foreground">{credits.remaining}</strong> AI credits left this month
    </span>
    <span class="text-xs text-muted-foreground">renews {fmtDate(credits.resets_at)}</span>
  </div>
{/if}
