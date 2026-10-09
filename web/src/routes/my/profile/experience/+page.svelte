<script lang="ts">
  import { api } from '$lib/api';
  import { askCvRefresh } from '$lib/cvRefreshDialog.svelte';
  import { BASE_REFRESH_MESSAGE, offerCvRefresh } from '$lib/cvRefreshOffer';
  import { errorMessage } from '$lib/utils';
  import ExperienceBankView from '$lib/components/ExperienceBankView.svelte';
  import { messages } from './messages';
  import { locale } from '$lib/i18n/currentLocale.svelte';
  import { t } from '$lib/i18n/t';

  const s = $derived(t(messages, locale()));

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
          await api.reseedBaseCv();
        } catch (e) {
          actionError = errorMessage(e, s.reseedFailed);
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
