<script lang="ts">
  import { api } from '$lib/api';
  import { askCvRefresh } from '$lib/cvRefreshDialog.svelte';
  import { BASE_REFRESH_MESSAGE, offerCvRefresh } from '$lib/cvRefreshOffer';
  import ExperienceBankView from '$lib/components/ExperienceBankView.svelte';

  // Scoped to this page rather than shared with sibling sections: a separate route
  // unmounts on navigation, so a stale error from one bank edit cannot keep showing
  // once the visitor leaves — no manual reset effect needed.
  let actionError = $state<string | null>(null);

  function offerRefreshAfterBankEdit() {
    void offerCvRefresh({
      message: BASE_REFRESH_MESSAGE,
      confirm: askCvRefresh,
      apply: async () => {
        // Cleared on the way in, so a failure from one edit does not outlive the next one that
        // succeeds — the banner sits above the bank and nothing else would ever drop it.
        actionError = null;
        try {
          await api.resetBaseCvFromResume();
        } catch {
          actionError = 'Could not update your base CV. Try Reset from résumé in a tailoring workspace.';
        }
      },
    });
  }
</script>

{#if actionError}
  <p class="mb-4 text-sm text-destructive">{actionError}</p>
{/if}

<!-- What the product has recorded about what this person has done, and the only
     place they can correct or remove it. -->
<ExperienceBankView onBankMutated={offerRefreshAfterBankEdit} />
