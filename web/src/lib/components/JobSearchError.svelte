<script lang="ts">
  import { resolve } from '$app/paths';
  import { Button, EmptyState } from '#lib/ui/index.js';
  import States from './States.svelte';
  import { searchRateLimit } from '#lib/searchRateLimit.js';

  // The caller retries the current query and page; the upgrade opens separately.
  let {
    error,
    onRetry,
    onlyRateLimit = false,
  }: { error: unknown; onRetry: () => void; onlyRateLimit?: boolean } = $props();
  const notice = $derived(searchRateLimit(error));
</script>

{#if notice}
  <EmptyState
    title={notice.title}
    description={notice.description}
    class="rounded-xl border border-dashed border-border"
    action={actions}
  />
{:else if !onlyRateLimit}
  <States state="error" message="Failed to load jobs." {onRetry} />
{/if}

{#snippet actions()}
  <div class="flex flex-wrap items-center justify-center gap-3">
    {#if notice?.upgrade}
      <Button variant="cta" size="sm" href={resolve('/pricing')} target="_blank" rel="noopener noreferrer"
        >Upgrade for higher search limits</Button
      >
    {/if}
    <Button variant="outline" size="sm" onclick={onRetry}>Retry search</Button>
  </div>
{/snippet}
