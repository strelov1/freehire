<script lang="ts">
  import Button from './button.svelte';

  let {
    loading,
    error = false,
    onclick,
  }: { loading: boolean; error?: boolean; onclick: () => void } = $props();
</script>

<div class="mt-6 flex flex-col items-center gap-2">
  <Button variant="outline" {onclick} disabled={loading}>
    {loading ? 'Loading…' : 'Load more'}
  </Button>
  {#if error}
    <p class="text-xs text-destructive">Couldn't load more. Try again.</p>
  {/if}
</div>
