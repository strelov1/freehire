<script lang="ts">
  // The unread count pill, shared by the messaging dock and the conversation rows.
  let { count }: { count: number } = $props();
</script>

{#if count}<span class="shrink-0 rounded-full bg-brand px-2 text-xs text-brand-foreground">{count}<span class="sr-only"> unread</span></span>{/if}
