<script lang="ts">
  // A dialog only makes sense next to the thing that opens it: `open` is
  // bindable, and a story that hard-codes it true cannot be reopened once
  // closed. So the trigger lives here and the story drives the content.
  import Button from '../../src/button.svelte';
  import Dialog from '../../src/dialog.svelte';

  let {
    title = 'Withdraw application?',
    description,
    dismissible = true,
    // 'long' stands in for a tall consumer (e.g. AuthDialog's form) to show the
    // mobile takeover actually scrolling, with the close button pinned in place.
    body = 'short',
  }: {
    title?: string;
    description?: string;
    dismissible?: boolean;
    body?: 'short' | 'long';
  } = $props();

  let open = $state(false);
</script>

<Button onclick={() => (open = true)}>Open dialog</Button>

<Dialog bind:open {title} {description} {dismissible}>
  {#if body === 'long'}
    <div class="space-y-4">
      {#each Array(12) as _, i (i)}
        <p class="text-sm text-muted-foreground">
          Paragraph {i + 1}. Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do
          eiusmod tempor incididunt ut labore et dolore magna aliqua.
        </p>
      {/each}
    </div>
    <div class="mt-6 flex justify-end gap-2">
      <Button variant="ghost" onclick={() => (open = false)}>Cancel</Button>
      <Button onclick={() => (open = false)}>Submit</Button>
    </div>
  {:else}
    <p class="text-sm text-muted-foreground">
      Withdrawing moves the application to Closed. You can apply again while the job is open.
    </p>
    <div class="mt-6 flex justify-end gap-2">
      <Button variant="ghost" onclick={() => (open = false)}>Cancel</Button>
      <Button variant="destructive" onclick={() => (open = false)}>Withdraw</Button>
    </div>
  {/if}
</Dialog>
