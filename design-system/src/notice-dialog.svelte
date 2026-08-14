<script lang="ts">
  import type { Snippet } from 'svelte';
  import Button from './button.svelte';
  import Dialog from './dialog.svelte';

  // The replacement for window.alert(): purely informational, one way out. No
  // busy/error handling like ConfirmDialog — there is nothing to await, so
  // Dialog's own default dismissible={true} is exactly right here.
  let {
    open = $bindable(false),
    title,
    description,
    confirmLabel = 'OK',
    children,
  }: {
    open?: boolean;
    title: string;
    description?: string;
    confirmLabel?: string;
    children?: Snippet;
  } = $props();
</script>

<Dialog bind:open {title} {description}>
  {#if children}
    {@render children()}
  {/if}
  <div class="mt-5 flex justify-end">
    <Button size="sm" onclick={() => (open = false)}>{confirmLabel}</Button>
  </div>
</Dialog>
