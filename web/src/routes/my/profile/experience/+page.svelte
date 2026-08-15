<script lang="ts">
  import { BASE_REFRESH_MESSAGE, offerCvRefresh } from '$lib/cvRefreshOffer';
  import { askCvRefresh } from '$lib/cvRefreshDialog.svelte';
  import { api } from '$lib/api';
  import ExperienceBankView from '$lib/components/ExperienceBankView.svelte';

  // What the product has recorded about what this person has done, and the only place
  // they can correct or remove it.
  let actionError = $state<string | null>(null);

  function offerRefreshAfterBankEdit() {
    void offerCvRefresh({
      message: BASE_REFRESH_MESSAGE,
      confirm: askCvRefresh,
      apply: async () => {
        // Cleared on the way in, so a failure from one edit does not outlive the next one
        // that succeeds — nothing else would ever drop the banner.
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

<svelte:head>
  <title>Profile experience — freehire</title>
</svelte:head>

{#if actionError}
  <p class="mb-4 text-sm text-destructive">{actionError}</p>
{/if}

<ExperienceBankView onBankMutated={offerRefreshAfterBankEdit} />
