<script lang="ts">
  // The one mount point for cvRefreshDialog.svelte.ts's controller — see there for why
  // this needs to exist as a singleton at all. Sibling of CookieConsent and SupportToast
  // in the root layout: fixed, self-gating, owns no layout space of its own.
  import { ConfirmDialog } from '$lib/ui';
  import { cvRefreshDialog, settleCvRefreshDialog } from '$lib/cvRefreshDialog.svelte';
</script>

<ConfirmDialog
  bind:open={() => cvRefreshDialog.open, (v) => { if (!v) settleCvRefreshDialog(false); }}
  title={cvRefreshDialog.message}
  confirmLabel="Update"
  onConfirm={() => settleCvRefreshDialog(true)}
/>
