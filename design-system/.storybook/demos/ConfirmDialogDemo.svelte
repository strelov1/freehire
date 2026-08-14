<script lang="ts">
  // Same reason as DialogDemo: `open` is bindable, so the trigger has to live
  // here for the story to be reopenable after a confirm or a cancel.
  import Button from '../../src/button.svelte';
  import ConfirmDialog from '../../src/confirm-dialog.svelte';
  import type { ButtonVariant } from '../../src/button.svelte';

  let {
    title = 'Delete saved search "Remote Go roles"?',
    description,
    confirmLabel = 'Delete',
    cancelLabel = 'Cancel',
    variant = 'destructive',
    fails = false,
  }: {
    title?: string;
    description?: string;
    confirmLabel?: string;
    cancelLabel?: string;
    variant?: ButtonVariant;
    /** Simulates the confirmed action rejecting, so the dialog's own error state is visible. */
    fails?: boolean;
  } = $props();

  let open = $state(false);

  // Stands in for an API call: a real call site's onConfirm awaits its own
  // request and lets a thrown ApiError surface through the dialog the same way.
  function onConfirm() {
    return new Promise<void>((resolve, reject) => {
      setTimeout(() => {
        if (fails) reject(new Error('Could not delete. Please try again.'));
        else resolve();
      }, 600);
    });
  }
</script>

<Button onclick={() => (open = true)}>Open confirm dialog</Button>

<ConfirmDialog bind:open {title} {description} {confirmLabel} {cancelLabel} {variant} {onConfirm} />
