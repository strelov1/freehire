<script lang="ts">
  // A dialog only makes sense next to the thing that opens it: `open` is
  // bindable, and a story that hard-codes it true cannot be reopened once
  // closed. So the trigger lives here and the story drives the content.
  import Button from '../../src/button.svelte';
  import Dialog from '../../src/dialog.svelte';

  let {
    title = 'Withdraw application?',
    description,
  }: { title?: string; description?: string } = $props();

  let open = $state(false);
</script>

<Button onclick={() => (open = true)}>Open dialog</Button>

<Dialog bind:open {title} {description}>
  <p class="text-sm text-muted-foreground">
    Withdrawing moves the application to Closed. You can apply again while the job is open.
  </p>
  <div class="mt-6 flex justify-end gap-2">
    <Button variant="ghost" onclick={() => (open = false)}>Cancel</Button>
    <Button variant="destructive" onclick={() => (open = false)}>Withdraw</Button>
  </div>
</Dialog>
