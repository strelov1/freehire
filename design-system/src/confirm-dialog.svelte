<script lang="ts">
  import type { Snippet } from 'svelte';
  import Button, { type ButtonVariant } from './button.svelte';
  import Dialog from './dialog.svelte';

  let {
    open = $bindable(false),
    title,
    description,
    confirmLabel = 'Confirm',
    cancelLabel = 'Cancel',
    // 'destructive' is the filled red button — reserve it the way Button itself
    // does: the action that destroys something and cannot be undone, not every
    // removal. A reversible one reads better as 'primary'.
    variant = 'primary',
    onConfirm,
    children,
  }: {
    open?: boolean;
    title: string;
    description?: string;
    confirmLabel?: string;
    cancelLabel?: string;
    variant?: ButtonVariant;
    /** May throw; its message is shown in place and the dialog stays open. */
    onConfirm: () => void | Promise<void>;
    children?: Snippet;
  } = $props();

  let busy = $state(false);
  let error = $state<string | null>(null);

  // Reset for the next time this dialog opens, not on every close mid-animation —
  // hanging the reset off `open` (like DeleteAccountButton) rather than off the
  // buttons keeps it correct across all three of Dialog's own close paths.
  $effect(() => {
    if (open) return;
    busy = false;
    error = null;
  });

  async function confirm() {
    if (busy) return;
    busy = true;
    error = null;
    try {
      await onConfirm();
      // Order matters: Dialog reasserts itself in onclose while it believes
      // itself held, so `busy` (and therefore `dismissible`) must already be
      // false in the same flush as `open` going false — otherwise Dialog's own
      // close effect fires el.close(), sees !dismissible in its onclose, and
      // synchronously showModal()s again before this component's own busy-reset
      // effect ever runs.
      busy = false;
      open = false;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Something went wrong. Please try again.';
      busy = false;
    }
  }
</script>

<Dialog bind:open {title} {description} dismissible={!busy}>
  {#if children}
    {@render children()}
  {/if}
  {#if error}
    <p class="mt-3 text-sm text-destructive">{error}</p>
  {/if}
  <div class="mt-5 flex items-center justify-end gap-2">
    <Button variant="ghost" size="sm" disabled={busy} onclick={() => (open = false)}>
      {cancelLabel}
    </Button>
    <Button {variant} size="sm" disabled={busy} onclick={confirm}>
      {busy ? `${confirmLabel}…` : confirmLabel}
    </Button>
  </div>
</Dialog>
