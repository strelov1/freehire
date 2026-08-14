<script lang="ts">
  // Same reason as DialogDemo and ConfirmDialogDemo: `open` is bindable, so the
  // trigger lives here for the story to be reopenable after acknowledging it.
  import Button from '../../src/button.svelte';
  import NoticeDialog from '../../src/notice-dialog.svelte';

  let {
    title = 'A new version of freehire is available',
    description,
    confirmLabel = 'OK',
  }: { title?: string; description?: string; confirmLabel?: string } = $props();

  let open = $state(false);
</script>

<Button onclick={() => (open = true)}>Open notice</Button>

<NoticeDialog bind:open {title} {description} {confirmLabel} />
