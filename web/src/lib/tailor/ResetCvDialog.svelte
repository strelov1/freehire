<script lang="ts">
  import { ConfirmDialog } from '#lib/ui/index.js';

  // Rebuilding the CV's content from the current seed (a base: the current CV).
  let {
    open = $bindable(),
    baseMode,
    onConfirm,
  }: { open: boolean; baseMode: boolean; onConfirm: () => Promise<void> | void } = $props();
</script>

<ConfirmDialog
  bind:open
  title={baseMode
    ? 'Replace this CV’s content with your current CV?'
    : 'Rebuild this tailored CV from your current seed?'}
  description={baseMode
    ? 'Anything you or the assistant edited in this CV is replaced. Your template and typography stay; the edits can be undone from History.'
    : 'Your template and typography stay; content edits can be undone from History.'}
  confirmLabel={baseMode ? 'Replace content' : 'Reset'}
  {onConfirm}
/>
